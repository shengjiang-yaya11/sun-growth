/*
 * ============================================================
 *  【ESP32 端固件】烧录目标: ESP32-S3 CAM V1.1 开发板
 *  编译工具链: ESP-IDF v5.x + Xtensa gcc (不依赖电脑端软件)
 *  烧录方式: USB 连接开发板, 运行 idf.py flash
 * ============================================================
 *
 * security_bridge.cc - mbedTLS 安全模块 C++ 封装实现
 *
 * 直接调用纯 C 安全模块函数 (bio_security.h)
 * 从 NVS "provision" 命名空间加载设备 ID 和密钥
 */

#include "security_bridge.h"
#include "config.h"
#include "bio_security.h"
#include "nvs_flash.h"
#include "esp_log.h"
#include "esp_random.h"
#include <cstring>
#include <cstdio>

static const char* TAG = "SEC";

// 静态成员初始化
std::string SecurityBridge::deviceId_;
uint8_t SecurityBridge::deviceSecret_[32] = {0};
uint32_t SecurityBridge::nonceCounter_ = 0;
bool SecurityBridge::valid_ = false;

// Base64 编码表
static const char base64Table[] =
    "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";

// 十六进制字符转字节值
static int hexCharToVal(char c) {
    if (c >= '0' && c <= '9') return c - '0';
    if (c >= 'a' && c <= 'f') return c - 'a' + 10;
    if (c >= 'A' && c <= 'F') return c - 'A' + 10;
    return -1;
}

// 十六进制字符串转字节数组
static void hexToBytes(const char* hex, uint8_t* out, size_t outLen) {
    for (size_t i = 0; i < outLen; i++) {
        int hi = hexCharToVal(hex[i * 2]);
        int lo = hexCharToVal(hex[i * 2 + 1]);
        if (hi < 0 || lo < 0) { out[i] = 0; continue; }
        out[i] = (uint8_t)((hi << 4) | lo);
    }
}

void SecurityBridge::init() {
    // 从 NVS "provision" 命名空间读取配置
    nvs_handle_t handle;
    esp_err_t err = nvs_open(NVS_NAMESPACE_PROVISION, NVS_READONLY, &handle);
    if (err == ESP_OK) {
        // 读取设备 ID
        char idBuf[32] = {0};
        size_t idLen = sizeof(idBuf);
        if (nvs_get_str(handle, NVS_KEY_DEVICE_ID, idBuf, &idLen) == ESP_OK) {
            deviceId_ = idBuf;
        }

        // 读取密钥 (十六进制字符串, 64 字符 = 32 字节)
        char secretHex[65] = {0};
        size_t secretLen = sizeof(secretHex);
        if (nvs_get_str(handle, NVS_KEY_SECRET, secretHex, &secretLen) == ESP_OK) {
            hexToBytes(secretHex, deviceSecret_, 32);
        }

        nvs_close(handle);
        ESP_LOGI(TAG, "配置已从 NVS 加载");
    }

    // 检查设备 ID 是否有效
    if (deviceId_.empty()) {
        ESP_LOGE(TAG, "NVS 中未找到设备 ID, 请先配网");
    }

    // 检查密钥是否非零
    bool keyValid = false;
    for (int i = 0; i < 32; i++) {
        if (deviceSecret_[i] != 0) { keyValid = true; break; }
    }
    if (!keyValid) {
        ESP_LOGE(TAG, "NVS 中未找到有效密钥, 请先配网");
    }

    // 仅当设备 ID 和密钥均有效时才标记为已初始化
    // 防止使用全零密钥进行加密/签名 (安全红线)
    valid_ = !deviceId_.empty() && keyValid;

    if (valid_) {
        ESP_LOGI(TAG, "安全模块初始化完成, 设备: %s", deviceId_.c_str());
    } else {
        ESP_LOGE(TAG, "安全模块未就绪, 加密/签名功能将拒绝执行");
    }
}

const std::string& SecurityBridge::getDeviceId() {
    return deviceId_;
}

const uint8_t* SecurityBridge::getDeviceSecret() {
    return deviceSecret_;
}

size_t SecurityBridge::getDeviceSecretLen() {
    return 32;
}

std::string SecurityBridge::encrypt(const uint8_t* plaintext, size_t len) {
    // 安全检查: 密钥未加载时拒绝加密 (防止使用全零密钥)
    if (!valid_) {
        ESP_LOGE(TAG, "加密拒绝: 安全模块未初始化");
        return "";
    }

    // 生成 nonce
    uint8_t nonce[12];
    generateNonce(nonce);

    // 分配输出缓冲区
    size_t outLen = len + SECURITY_OVERHEAD;
    std::string output;
    output.resize(outLen);

    // 调用纯 C 安全模块的加密函数
    int written = bio_encrypt(
        plaintext, len,
        deviceSecret_, 32,
        nonce,
        reinterpret_cast<uint8_t*>(output.data()), outLen
    );

    if (written <= 0) {
        ESP_LOGE(TAG, "加密失败");
        return "";
    }

    output.resize(written);
    return output;
}

void SecurityBridge::sha256(const uint8_t* data, size_t len, uint8_t out[32]) {
    bio_sha256(data, len, out);
}

std::string SecurityBridge::signRequest(
    const std::string& timestamp,
    const std::string& nonce,
    const uint8_t payloadHash[32]
) {
    // 安全检查: 密钥未加载时拒绝签名
    if (!valid_) {
        ESP_LOGE(TAG, "签名拒绝: 安全模块未初始化");
        return "";
    }

    uint8_t sig[SECURITY_SIG_LEN];
    int sigLen = bio_sign_request(
        deviceSecret_, 32,
        reinterpret_cast<const uint8_t*>(deviceId_.data()), deviceId_.length(),
        reinterpret_cast<const uint8_t*>(timestamp.data()), timestamp.length(),
        reinterpret_cast<const uint8_t*>(nonce.data()), nonce.length(),
        payloadHash,
        sig, sizeof(sig)
    );

    if (sigLen <= 0) {
        ESP_LOGE(TAG, "签名失败");
        return "";
    }

    return base64Encode(sig, sigLen);
}

std::string SecurityBridge::base64Encode(const uint8_t* data, size_t len) {
    std::string result;
    result.reserve(((len + 2) / 3) * 4);

    for (size_t i = 0; i < len; i += 3) {
        uint32_t n = (uint32_t)data[i] << 16;
        if (i + 1 < len) n |= (uint32_t)data[i + 1] << 8;
        if (i + 2 < len) n |= (uint32_t)data[i + 2];

        result += base64Table[(n >> 18) & 0x3F];
        result += base64Table[(n >> 12) & 0x3F];
        result += (i + 1 < len) ? base64Table[(n >> 6) & 0x3F] : '=';
        result += (i + 2 < len) ? base64Table[n & 0x3F] : '=';
    }

    return result;
}

void SecurityBridge::generateNonce(uint8_t nonce[12]) {
    // 前 8 字节使用硬件随机数, 保证跨重启唯一性
    // (esp_timer_get_time() 在重启后归零, 会导致 nonce 重复)
    esp_fill_random(nonce, 8);

    // 后 4 字节使用递增计数器, 保证同一会话内唯一
    nonceCounter_++;
    memcpy(nonce + 8, &nonceCounter_, 4);
}

std::string SecurityBridge::generateNonceHex() {
    uint8_t nonce[12];
    generateNonce(nonce);

    char hex[25];
    for (int i = 0; i < 12; i++) {
        snprintf(hex + i * 2, 3, "%02x", nonce[i]);
    }
    return std::string(hex, 24);
}
