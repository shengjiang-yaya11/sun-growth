/*
 * ============================================================
 *  【ESP32 端固件】烧录目标: ESP32-S3 CAM V1.1 开发板
 *  编译工具链: ESP-IDF v5.x + Xtensa gcc (不依赖电脑端软件)
 *  烧录方式: USB 连接开发板, 运行 idf.py flash
 * ============================================================
 *
 * security_bridge.h - mbedTLS 安全模块的 C++ 封装
 *
 * 将纯 C 安全模块 (bio_security.h) 封装为 C++ 友好接口
 * 负责: 密钥管理、加密、签名、Base64 编码
 *
 * 安全模块底层使用 ESP-IDF 内置 mbedTLS (ADR-001)
 */

#pragma once
#include <stdint.h>
#include <stddef.h>
#include <string>

class SecurityBridge {
public:
    /// 初始化, 从 NVS 加载设备 ID 和密钥
    static void init();

    /// 获取设备 ID
    static const std::string& getDeviceId();

    /// 获取设备密钥 (32 字节)
    static const uint8_t* getDeviceSecret();
    static size_t getDeviceSecretLen();

    /// 加密数据 (Encrypt-then-MAC)
    /// 返回加密后数据, 失败返回空
    static std::string encrypt(const uint8_t* plaintext, size_t len);

    /// 计算 SHA-256 哈希
    static void sha256(const uint8_t* data, size_t len, uint8_t out[32]);

    /// 生成请求签名
    /// sig = HMAC(device_secret, device_id || timestamp || nonce || payload_hash)
    static std::string signRequest(const std::string& timestamp,
                                   const std::string& nonce,
                                   const uint8_t payloadHash[32]);

    /// Base64 编码
    static std::string base64Encode(const uint8_t* data, size_t len);

    /// 生成 nonce (时间戳 8B + 计数器 4B = 12B 二进制)
    static void generateNonce(uint8_t nonce[12]);

    /// 生成 nonce 的十六进制字符串 (用于 HTTP 头)
    static std::string generateNonceHex();

    /// 检查安全模块是否已正确初始化 (设备 ID 和密钥均已加载)
    static bool isValid() { return valid_; }

private:
    static std::string deviceId_;
    static uint8_t deviceSecret_[32];
    static uint32_t nonceCounter_;
    static bool valid_;  ///< 密钥和设备 ID 是否已成功加载
};
