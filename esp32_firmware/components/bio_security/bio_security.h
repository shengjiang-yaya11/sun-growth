/*
 * ============================================================
 *  【ESP32 端固件】烧录目标: ESP32-S3 CAM V1.1 开发板
 *  编译工具链: ESP-IDF v5.x + Xtensa gcc (不依赖电脑端软件)
 *  烧录方式: USB 连接开发板, 运行 idf.py flash
 * ============================================================
 *
 * bio_security.h - 纯 C 安全模块头文件 (ESP-IDF mbedTLS 实现)
 *
 * 安全协议:
 *   1. 设备持有 32 字节预共享密钥 (device_secret)
 *   2. 每次上传:
 *      a. 生成 nonce (12 字节, 时间戳+计数器)
 *      b. AES-256-CTR 加密 JPEG (密钥由 device_secret 派生)
 *      c. HMAC-SHA256 认证 (Encrypt-then-MAC)
 *      d. HMAC-SHA256 签名请求头 (device_id + timestamp + nonce + payload_hash)
 *
 * 密钥派生:
 *   enc_key = HMAC(device_secret, "bio-enc-key-v1")
 *   mac_key = HMAC(device_secret, "bio-mac-key-v1")
 *
 * 密文格式: [nonce(12)] [hmac_tag(32)] [ciphertext(N)]
 *
 * 替代原 Rust no_std 安全模块 (ADR-001)
 */

#ifndef BIO_SECURITY_H
#define BIO_SECURITY_H

#include <stdint.h>
#include <stddef.h>

#ifdef __cplusplus
extern "C" {
#endif

/* 安全协议常量 */
#define BIO_NONCE_LEN   12   /* CTR 模式 IV (12 字节 nonce) */
#define BIO_TAG_LEN     32   /* HMAC-SHA256 标签长度 */
#define BIO_HASH_LEN    32   /* SHA-256 哈希长度 */
#define BIO_SIG_LEN     32   /* 签名长度 */
#define BIO_OVERHEAD    44   /* 加密开销 = nonce(12) + tag(32) */
#define BIO_KEY_LEN     32   /* 密钥长度 (256 位) */

/*
 * 加密数据 (Encrypt-then-MAC: AES-256-CTR + HMAC-SHA256)
 *
 * 密钥派生:
 *   enc_key = HMAC(secret, "bio-enc-key-v1")
 *   mac_key = HMAC(secret, "bio-mac-key-v1")
 *
 * 参数:
 *   plaintext   - 明文数据指针 (不可为 NULL)
 *   pt_len      - 明文长度
 *   secret      - 设备密钥 (不可为 NULL)
 *   secret_len  - 密钥长度
 *   nonce       - 12 字节 nonce (不可为 NULL)
 *   output      - 输出缓冲区, 大小 >= pt_len + BIO_OVERHEAD (不可为 NULL)
 *   out_len     - 输出缓冲区大小
 *
 * 返回: 写入字节数 (>0 成功), 0 = 错误
 * 输出格式: [nonce(12)] [hmac_tag(32)] [ciphertext(N)]
 */
int bio_encrypt(const uint8_t *plaintext, size_t pt_len,
                const uint8_t *secret, size_t secret_len,
                const uint8_t *nonce,
                uint8_t *output, size_t out_len);

/*
 * 计算 SHA-256 哈希
 *
 * 参数:
 *   data    - 输入数据 (不可为 NULL, len 可为 0)
 *   len     - 数据长度
 *   out_32  - 输出 32 字节哈希 (不可为 NULL)
 */
void bio_sha256(const uint8_t *data, size_t len, uint8_t *out_32);

/*
 * 计算 HMAC-SHA256
 *
 * 参数:
 *   key       - HMAC 密钥 (不可为 NULL)
 *   key_len   - 密钥长度
 *   data      - 输入数据 (不可为 NULL, len 可为 0)
 *   data_len  - 数据长度
 *   out_32    - 输出 32 字节 HMAC (不可为 NULL)
 *
 * 返回: 0 = 成功, 非 0 = 失败
 */
int bio_hmac_sha256(const uint8_t *key, size_t key_len,
                    const uint8_t *data, size_t data_len,
                    uint8_t *out_32);

/*
 * 生成请求签名 (HMAC-SHA256)
 *
 * 签名 = HMAC(secret, device_id || \0 || timestamp || \0 || nonce || \0 || payload_hash)
 *
 * 参数:
 *   secret         - 设备密钥 (不可为 NULL)
 *   secret_len     - 密钥长度
 *   device_id      - 设备 ID 字符串 (不可为 NULL)
 *   id_len         - 设备 ID 长度
 *   timestamp      - 时间戳字符串 ASCII (不可为 NULL)
 *   ts_len         - 时间戳长度
 *   nonce          - nonce 字符串 ASCII (不可为 NULL)
 *   nonce_len      - nonce 长度
 *   payload_hash   - 载荷 SHA-256 哈希, 32 字节 (不可为 NULL)
 *   sig_out        - 输出签名缓冲区, >= 32 字节 (不可为 NULL)
 *   sig_out_len    - 输出缓冲区大小
 *
 * 返回: 签名长度 (32), 0 = 错误
 */
int bio_sign_request(const uint8_t *secret, size_t secret_len,
                     const uint8_t *device_id, size_t id_len,
                     const uint8_t *timestamp, size_t ts_len,
                     const uint8_t *nonce, size_t nonce_len,
                     const uint8_t *payload_hash,
                     uint8_t *sig_out, size_t sig_out_len);

/*
 * 获取加密开销字节数 (nonce + tag = 44)
 *
 * 返回: BIO_OVERHEAD
 */
int bio_get_overhead(void);

/*
 * 获取密钥长度 (32 字节)
 *
 * 返回: BIO_KEY_LEN
 */
int bio_get_key_len(void);

#ifdef __cplusplus
}
#endif

#endif /* BIO_SECURITY_H */
