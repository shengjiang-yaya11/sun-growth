/*
 * ============================================================
 *  【ESP32 端固件】烧录目标: ESP32-S3 CAM V1.1 开发板
 *  编译工具链: ESP-IDF v5.x + Xtensa gcc (不依赖电脑端软件)
 *  烧录方式: USB 连接开发板, 运行 idf.py flash
 * ============================================================
 *
 * uploader.cc - 安全上传器实现
 *
 * 流程:
 *   1. 拍照获取 JPEG
 *   2. SHA-256 计算载荷哈希
 *   3. AES-256-CTR 加密 JPEG (Encrypt-then-MAC)
 *   4. HMAC-SHA256 签名请求头 (device_id + timestamp + nonce + payload_hash)
 *   5. HTTP POST 发送加密数据 + 签名头
 *   6. 服务器验证签名 -> 解密 -> 存储
 *
 * ADR-007: 上传失败时将加密数据缓存到 PSRAM 重试队列, 主循环重试
 */

#include "uploader.h"
#include "security_bridge.h"
#include "config.h"
#include "esp_http_client.h"
#include "esp_crt_bundle.h"
#include "esp_log.h"
#include "esp_timer.h"
#include "nvs_flash.h"
#include "time.h"
#include "sys/time.h"
#include <cstring>
#include <cstdio>

static const char* TAG = "UPLOAD";

std::string Uploader::buildUrl() {
    // 从 NVS "provision" 命名空间读取服务器配置
    std::string host;
    uint16_t port = PROVISION_DEFAULT_PORT;
    std::string path = DEFAULT_SERVER_PATH;
    bool useHttps = false;

    nvs_handle_t handle;
    if (nvs_open(NVS_NAMESPACE_PROVISION, NVS_READONLY, &handle) == ESP_OK) {
        char buf[128] = {0};
        size_t len = sizeof(buf);
        if (nvs_get_str(handle, NVS_KEY_HOST, buf, &len) == ESP_OK) host = buf;
        uint16_t p;
        if (nvs_get_u16(handle, NVS_KEY_PORT, &p) == ESP_OK) port = p;
        uint8_t https = 0;
        if (nvs_get_u8(handle, NVS_KEY_USE_HTTPS, &https) == ESP_OK) useHttps = (https == 1);
        nvs_close(handle);
    }

    if (host.empty()) {
        ESP_LOGE(TAG, "NVS 中未找到服务器配置, 请先配网");
        return "";
    }

    const char* protocol = useHttps ? "https" : "http";
    char url[256];
    snprintf(url, sizeof(url), "%s://%s:%d%s", protocol, host.c_str(), port, path.c_str());
    return std::string(url);
}

std::string Uploader::getTimestamp() {
    // 使用 SNTP 同步的时间, 如果未同步则用 esp_timer
    time_t now;
    time(&now);
    if (now < 1700000000) {
        // 时间未同步, 使用启动以来的微秒估算
        now = 1700000000 + (esp_timer_get_time() / 1000000);
    }
    char buf[16];
    snprintf(buf, sizeof(buf), "%lld", (long long)now);
    return std::string(buf);
}

bool Uploader::performUpload(const std::string& url,
                              const std::string& deviceId,
                              const std::string& timestamp,
                              const std::string& nonce,
                              const std::string& signature,
                              const std::string& hashB64,
                              const uint8_t* encryptedData,
                              size_t encryptedLen) {
    if (url.empty()) {
        ESP_LOGE(TAG, "URL 为空");
        return false;
    }

    esp_http_client_config_t config = {};
    config.url = url.c_str();
    config.method = HTTP_METHOD_POST;
    config.timeout_ms = HTTP_TIMEOUT_MS;
    config.disable_auto_redirect = false;
    config.buffer_size = 2048;
    config.buffer_size_tx = 4096;
    // 启用 TLS 证书验证 (HTTPS 时生效, HTTP 时忽略)
    // 防止中间人攻击, 确保连接到真正的服务器
    config.crt_bundle_attach = esp_crt_bundle_attach;

    esp_http_client_handle_t client = esp_http_client_init(&config);
    if (!client) {
        ESP_LOGE(TAG, "HTTP 客户端初始化失败");
        return false;
    }

    // 设置安全头
    esp_http_client_set_header(client, "Content-Type", "application/octet-stream");
    esp_http_client_set_header(client, "X-Device-ID", deviceId.c_str());
    esp_http_client_set_header(client, "X-Timestamp", timestamp.c_str());
    esp_http_client_set_header(client, "X-Nonce", nonce.c_str());
    esp_http_client_set_header(client, "X-Signature", signature.c_str());
    esp_http_client_set_header(client, "X-Payload-Hash", hashB64.c_str());
    esp_http_client_set_header(client, "X-Firmware-Version", FIRMWARE_VERSION);

    // 设置 POST 数据 (加密后的)
    esp_http_client_set_post_field(client, (const char*)encryptedData, encryptedLen);

    esp_err_t err = esp_http_client_perform(client);
    bool success = false;

    if (err == ESP_OK) {
        int status = esp_http_client_get_status_code(client);
        if (status == 200 || status == 201) {
            ESP_LOGI(TAG, "上传成功 (HTTP %d)", status);
            success = true;
        } else {
            ESP_LOGE(TAG, "上传失败 (HTTP %d)", status);
            char resp[256] = {0};
            int readLen = esp_http_client_read(client, resp, sizeof(resp) - 1);
            if (readLen > 0) ESP_LOGE(TAG, "响应: %s", resp);
        }
    } else {
        ESP_LOGE(TAG, "HTTP 错误: %s", esp_err_to_name(err));
    }

    esp_http_client_cleanup(client);
    return success;
}

bool Uploader::upload(const uint8_t* jpeg, size_t len) {
    if (!jpeg || len == 0) {
        ESP_LOGE(TAG, "无效输入");
        return false;
    }

    // 1. 计算 SHA-256 载荷哈希
    uint8_t payloadHash[SECURITY_HASH_LEN];
    SecurityBridge::sha256(jpeg, len, payloadHash);

    // 2. 加密 JPEG
    std::string encrypted = SecurityBridge::encrypt(jpeg, len);
    if (encrypted.empty()) {
        ESP_LOGE(TAG, "加密失败");
        return false;
    }

    // 3. 生成时间戳和 nonce
    std::string timestamp = getTimestamp();
    std::string nonce = SecurityBridge::generateNonceHex();

    // 4. 签名请求
    std::string signature = SecurityBridge::signRequest(timestamp, nonce, payloadHash);

    // 载荷哈希的 Base64 (用于 HTTP 头)
    std::string hashB64 = SecurityBridge::base64Encode(payloadHash, 32);

    // 5. 获取 URL 和设备 ID
    std::string url = buildUrl();
    std::string deviceId = SecurityBridge::getDeviceId();

    ESP_LOGI(TAG, "上传: %zu B -> 加密 %zu B -> %s",
             len, encrypted.size(), url.c_str());

    // 6. 执行 HTTP POST
    bool success = performUpload(url, deviceId, timestamp, nonce,
                                  signature, hashB64,
                                  (const uint8_t*)encrypted.data(),
                                  encrypted.size());

    if (!success) {
        // ADR-007: 上传失败, 缓存原始明文到 PSRAM 重试队列
        // 重试时重新加密+签名, 避免 nonce 重放和时间戳过期
        RetryQueue::Entry entry;
        entry.plaintextData.assign((const char*)jpeg, len);
        entry.deviceId      = deviceId;
        entry.originalLen   = (uint32_t)len;
        entry.retryCount    = 0;

        if (RetryQueue::push(entry)) {
            ESP_LOGI(TAG, "上传失败, 已缓存到重试队列 (将在主循环中重试)");
        } else {
            ESP_LOGE(TAG, "上传失败且缓存失败, 数据丢失");
        }
    }

    return success;
}

bool Uploader::retryCached() {
    RetryQueue::Entry entry;
    if (!RetryQueue::pop(entry)) {
        return false;  // 队列为空
    }

    // 检查重试次数
    if (entry.retryCount >= MAX_RETRY_ATTEMPTS) {
        ESP_LOGW(TAG, "照片重试次数已达上限 (%d), 丢弃",
                 (int)MAX_RETRY_ATTEMPTS);
        return false;
    }

    // 检查超时
    uint32_t now = (uint32_t)(esp_timer_get_time() / 1000000);
    if (now - entry.createdAt > RETRY_TIMEOUT_SECONDS) {
        ESP_LOGW(TAG, "照片缓存已超时 (%u 秒), 丢弃",
                 (unsigned)(now - entry.createdAt));
        return false;
    }

    entry.retryCount++;
    ESP_LOGI(TAG, "重试上传 (第 %d 次, 原始 %u B)",
             (int)entry.retryCount, (unsigned)entry.originalLen);

    // 重新加密 + 签名 (使用新的 nonce 和时间戳, 防止重放攻击和时间戳过期)
    const uint8_t* plaintext = (const uint8_t*)entry.plaintextData.data();
    size_t plaintextLen = entry.plaintextData.size();

    // 1. 计算 SHA-256 载荷哈希
    uint8_t payloadHash[SECURITY_HASH_LEN];
    SecurityBridge::sha256(plaintext, plaintextLen, payloadHash);

    // 2. 重新加密
    std::string reEncrypted = SecurityBridge::encrypt(plaintext, plaintextLen);
    if (reEncrypted.empty()) {
        ESP_LOGE(TAG, "重试: 重新加密失败");
        // 重新入队, 下次再试
        if (entry.retryCount < MAX_RETRY_ATTEMPTS) {
            RetryQueue::push(entry);
        }
        return false;
    }

    // 3. 生成新的时间戳和 nonce
    std::string newTimestamp = getTimestamp();
    std::string newNonce = SecurityBridge::generateNonceHex();

    // 4. 重新签名
    std::string newSignature = SecurityBridge::signRequest(newTimestamp, newNonce, payloadHash);
    std::string newHashB64 = SecurityBridge::base64Encode(payloadHash, 32);

    // 5. 重新构建 URL (服务器地址可能在配网后变化)
    std::string url = buildUrl();

    bool success = performUpload(url, entry.deviceId, newTimestamp,
                                  newNonce, newSignature, newHashB64,
                                  (const uint8_t*)reEncrypted.data(),
                                  reEncrypted.size());

    if (!success) {
        // 重试仍失败, 重新入队 (如果队列未满)
        if (entry.retryCount < MAX_RETRY_ATTEMPTS) {
            RetryQueue::push(entry);
            ESP_LOGW(TAG, "重试失败, 已重新入队 (剩余重试: %d)",
                     (int)(MAX_RETRY_ATTEMPTS - entry.retryCount));
        } else {
            ESP_LOGE(TAG, "重试失败且已达上限, 丢弃照片");
        }
    }

    return success;
}

void Uploader::processRetries() {
    if (RetryQueue::empty()) {
        return;
    }

    ESP_LOGI(TAG, "处理重试队列 (%d 条待重试)", (int)RetryQueue::size());

    size_t processed = 0;
    while (processed < MAX_RETRIES_PER_CYCLE && !RetryQueue::empty()) {
        retryCached();
        processed++;
    }
}

size_t Uploader::getPendingRetryCount() {
    return RetryQueue::size();
}
