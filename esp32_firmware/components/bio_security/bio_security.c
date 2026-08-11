/*
 * ============================================================
 *  【ESP32 端固件】烧录目标: ESP32-S3 CAM V1.1 开发板
 *  编译工具链: ESP-IDF v5.x + Xtensa gcc (不依赖电脑端软件)
 *  烧录方式: USB 连接开发板, 运行 idf.py flash
 * ============================================================
 *
 * bio_security.c - 纯 C 安全模块实现 (ESP-IDF mbedTLS)
 *
 * 实现 AES-256-CTR 加密 + HMAC-SHA256 签名 + SHA-256 哈希
 * 使用 ESP-IDF 内置 mbedTLS 库, 无外部依赖
 *
 * 安全协议:
 *   加密: AES-256-CTR, 密钥由 HMAC(device_secret, label) 派生
 *   认证: HMAC-SHA256(mac_key, nonce || ciphertext) — Encrypt-then-MAC
 *   签名: HMAC-SHA256(device_secret, device_id || \0 || ts || \0 || nonce || \0 || hash)
 *
 * 密文格式: [nonce(12)] [hmac_tag(32)] [ciphertext(N)]
 *
 * 替代原 Rust no_std 安全模块 (ADR-001)
 */

#include "bio_security.h"

#include "mbedtls/version.h"
#include "mbedtls/md.h"
#include "mbedtls/aes.h"
#include "mbedtls/sha256.h"

#include <string.h>

/* ============================================================
 *  mbedTLS 跨版本兼容层
 *
 *  mbedTLS 2.x (ESP-IDF v5.0-v5.1): _ret 后缀函数返回 int,
 *                                    无后缀函数返回 void
 *  mbedTLS 3.x (ESP-IDF v5.2+):    无后缀函数返回 int,
 *                                    _ret 后缀已移除
 * ============================================================ */
#if MBEDTLS_VERSION_NUMBER < 0x03000000
  #define BIO_AES_SETKEY_ENC(ctx, key, bits) \
      mbedtls_aes_setkey_enc_ret((ctx), (key), (bits))
  #define BIO_AES_CRYPT_CTR(ctx, len, off, nc, sb, in, out) \
      mbedtls_aes_crypt_ctr_ret((ctx), (len), (off), (nc), (sb), (in), (out))
  #define BIO_SHA256(data, len, out, is224) \
      mbedtls_sha256_ret((data), (len), (out), (is224))
#else
  #define BIO_AES_SETKEY_ENC(ctx, key, bits) \
      mbedtls_aes_setkey_enc((ctx), (key), (bits))
  #define BIO_AES_CRYPT_CTR(ctx, len, off, nc, sb, in, out) \
      mbedtls_aes_crypt_ctr((ctx), (len), (off), (nc), (sb), (in), (out))
  #define BIO_SHA256(data, len, out, is224) \
      mbedtls_sha256((data), (len), (out), (is224))
#endif

/* ============================================================
 *  内部辅助函数
 * ============================================================ */

/*
 * 使用 HMAC-SHA256 从设备密钥派生子密钥
 *   out = HMAC(secret, label)  (32 字节)
 *
 * 参数:
 *   secret     - 设备密钥
 *   secret_len - 密钥长度
 *   label      - 派生标签 (以 \0 结尾的字符串)
 *   out        - 输出 32 字节子密钥
 *
 * 返回: 0 = 成功, -1 = 失败
 */
static int derive_key(const uint8_t *secret, size_t secret_len,
                      const char *label, uint8_t *out)
{
    const mbedtls_md_info_t *info = mbedtls_md_info_from_type(MBEDTLS_MD_SHA256);
    if (info == NULL) {
        return -1;
    }

    mbedtls_md_context_t ctx;
    mbedtls_md_init(&ctx);

    int ret = mbedtls_md_setup(&ctx, info, 1);  /* 1 = HMAC 模式 */
    if (ret != 0) {
        goto cleanup;
    }

    ret = mbedtls_md_hmac_starts(&ctx, secret, secret_len);
    if (ret != 0) {
        goto cleanup;
    }

    ret = mbedtls_md_hmac_update(&ctx, (const unsigned char *)label, strlen(label));
    if (ret != 0) {
        goto cleanup;
    }

    ret = mbedtls_md_hmac_finish(&ctx, out);

cleanup:
    mbedtls_md_free(&ctx);
    return ret;
}

/* ============================================================
 *  公开 API 实现
 * ============================================================ */

int bio_encrypt(const uint8_t *plaintext, size_t pt_len,
                const uint8_t *secret, size_t secret_len,
                const uint8_t *nonce,
                uint8_t *output, size_t out_len)
{
    /* 空指针检查 */
    if (plaintext == NULL || secret == NULL || nonce == NULL || output == NULL) {
        return 0;
    }

    /* 密钥长度检查 */
    if (secret_len == 0) {
        return 0;
    }

    /* 输出缓冲区大小检查 */
    size_t needed = pt_len + BIO_OVERHEAD;
    if (out_len < needed) {
        return 0;
    }

    /* 1. 派生加密密钥: enc_key = HMAC(secret, "bio-enc-key-v1") */
    uint8_t enc_key[BIO_KEY_LEN];
    if (derive_key(secret, secret_len, "bio-enc-key-v1", enc_key) != 0) {
        return 0;
    }

    /* 2. 写入 nonce 到输出前 12 字节 */
    memcpy(output, nonce, BIO_NONCE_LEN);

    /* 3. 复制明文到输出末尾 (密文区域), 就地加密 */
    uint8_t *ciphertext = output + BIO_NONCE_LEN + BIO_TAG_LEN;
    if (pt_len > 0) {
        memcpy(ciphertext, plaintext, pt_len);

        /* 4. AES-256-CTR 加密 */
        mbedtls_aes_context aes;
        mbedtls_aes_init(&aes);

        int ret = BIO_AES_SETKEY_ENC(&aes, enc_key, 256);
        if (ret != 0) {
            mbedtls_aes_free(&aes);
            return 0;
        }

        /* 构建 16 字节初始计数器块: [nonce(12)] [counter(4)=0] */
        unsigned char nonce_counter[16] = {0};
        memcpy(nonce_counter, nonce, BIO_NONCE_LEN);

        size_t nc_off = 0;
        unsigned char stream_block[16] = {0};

        ret = BIO_AES_CRYPT_CTR(&aes, pt_len, &nc_off,
                                nonce_counter, stream_block,
                                ciphertext, ciphertext);
        mbedtls_aes_free(&aes);

        if (ret != 0) {
            return 0;
        }
    }

    /* 5. 派生 MAC 密钥: mac_key = HMAC(secret, "bio-mac-key-v1") */
    uint8_t mac_key[BIO_KEY_LEN];
    if (derive_key(secret, secret_len, "bio-mac-key-v1", mac_key) != 0) {
        return 0;
    }

    /* 6. 计算 HMAC 标签 = HMAC(mac_key, nonce || ciphertext) */
    const mbedtls_md_info_t *info = mbedtls_md_info_from_type(MBEDTLS_MD_SHA256);
    if (info == NULL) {
        return 0;
    }

    mbedtls_md_context_t ctx;
    mbedtls_md_init(&ctx);

    int ret = mbedtls_md_setup(&ctx, info, 1);
    if (ret != 0) {
        mbedtls_md_free(&ctx);
        return 0;
    }

    ret = mbedtls_md_hmac_starts(&ctx, mac_key, BIO_KEY_LEN);
    if (ret != 0) {
        mbedtls_md_free(&ctx);
        return 0;
    }

    /* HMAC 覆盖 nonce */
    ret = mbedtls_md_hmac_update(&ctx, nonce, BIO_NONCE_LEN);
    if (ret != 0) {
        mbedtls_md_free(&ctx);
        return 0;
    }

    /* HMAC 覆盖密文 (可能为 0 长度) */
    if (pt_len > 0) {
        ret = mbedtls_md_hmac_update(&ctx, ciphertext, pt_len);
        if (ret != 0) {
            mbedtls_md_free(&ctx);
            return 0;
        }
    }

    /* 写入 HMAC 标签到输出 [12..44) */
    ret = mbedtls_md_hmac_finish(&ctx, output + BIO_NONCE_LEN);
    mbedtls_md_free(&ctx);

    if (ret != 0) {
        return 0;
    }

    /* 返回总写入字节数 */
    return (int)needed;
}

void bio_sha256(const uint8_t *data, size_t len, uint8_t *out_32)
{
    if (data == NULL || out_32 == NULL) {
        return;
    }

    /* mbedTLS SHA-256, 参数 0 表示使用 SHA-256 (非 SHA-224) */
    BIO_SHA256(data, len, out_32, 0);
}

int bio_sign_request(const uint8_t *secret, size_t secret_len,
                     const uint8_t *device_id, size_t id_len,
                     const uint8_t *timestamp, size_t ts_len,
                     const uint8_t *nonce, size_t nonce_len,
                     const uint8_t *payload_hash,
                     uint8_t *sig_out, size_t sig_out_len)
{
    /* 空指针检查 */
    if (secret == NULL || device_id == NULL || timestamp == NULL ||
        nonce == NULL || payload_hash == NULL || sig_out == NULL) {
        return 0;
    }

    /* 密钥长度检查 */
    if (secret_len == 0) {
        return 0;
    }

    /* 输出缓冲区检查 */
    if (sig_out_len < BIO_SIG_LEN) {
        return 0;
    }

    const mbedtls_md_info_t *info = mbedtls_md_info_from_type(MBEDTLS_MD_SHA256);
    if (info == NULL) {
        return 0;
    }

    mbedtls_md_context_t ctx;
    mbedtls_md_init(&ctx);

    int ret = mbedtls_md_setup(&ctx, info, 1);
    if (ret != 0) {
        mbedtls_md_free(&ctx);
        return 0;
    }

    /* HMAC 使用原始 device_secret 作为密钥 */
    ret = mbedtls_md_hmac_starts(&ctx, secret, secret_len);
    if (ret != 0) {
        mbedtls_md_free(&ctx);
        return 0;
    }

    /* 字段分隔符 \0 防止拼接歧义 */
    static const unsigned char sep = '\0';

    /* device_id || \0 */
    ret = mbedtls_md_hmac_update(&ctx, device_id, id_len);
    if (ret != 0) goto fail;
    ret = mbedtls_md_hmac_update(&ctx, &sep, 1);
    if (ret != 0) goto fail;

    /* timestamp || \0 */
    ret = mbedtls_md_hmac_update(&ctx, timestamp, ts_len);
    if (ret != 0) goto fail;
    ret = mbedtls_md_hmac_update(&ctx, &sep, 1);
    if (ret != 0) goto fail;

    /* nonce || \0 */
    ret = mbedtls_md_hmac_update(&ctx, nonce, nonce_len);
    if (ret != 0) goto fail;
    ret = mbedtls_md_hmac_update(&ctx, &sep, 1);
    if (ret != 0) goto fail;

    /* payload_hash (32 字节) */
    ret = mbedtls_md_hmac_update(&ctx, payload_hash, BIO_HASH_LEN);
    if (ret != 0) goto fail;

    /* 完成计算, 写入签名 */
    ret = mbedtls_md_hmac_finish(&ctx, sig_out);
    if (ret != 0) goto fail;

    mbedtls_md_free(&ctx);
    return BIO_SIG_LEN;

fail:
    mbedtls_md_free(&ctx);
    return 0;
}

int bio_get_overhead(void)
{
    return BIO_OVERHEAD;
}

int bio_get_key_len(void)
{
    return BIO_KEY_LEN;
}
