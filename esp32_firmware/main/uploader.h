/*
 * ============================================================
 *  【ESP32 端固件】烧录目标: ESP32-S3 CAM V1.1 开发板
 *  编译工具链: ESP-IDF v5.x + Xtensa gcc (不依赖电脑端软件)
 *  烧录方式: USB 连接开发板, 运行 idf.py flash
 * ============================================================
 *
 * uploader.h - 安全上传器
 *
 * 拍照 -> 加密 -> 签名 -> HTTP POST 推送
 *
 * ADR-007: 上传失败时将加密数据缓存到 PSRAM 重试队列
 */

#pragma once
#include <string>
#include <cstdint>
#include "retry_queue.h"

class Uploader {
public:
    /// 上传一张照片
    /// jpeg: JPEG 数据指针, len: 长度
    /// 返回: true=成功
    /// 失败时自动缓存到 RetryQueue
    static bool upload(const uint8_t* jpeg, size_t len);

    /// 重试队列中的缓存照片
    /// 返回: true=重试成功, false=重试失败或队列为空
    static bool retryCached();

    /// 处理重试队列 (主循环调用)
    /// 每次最多重试 MAX_RETRIES_PER_CYCLE 条
    static void processRetries();

    /// 获取待重试的照片数量
    static size_t getPendingRetryCount();

private:
    /// 构建上传 URL
    static std::string buildUrl();

    /// 获取当前 Unix 时间戳字符串
    static std::string getTimestamp();

    /// 执行 HTTP POST (核心上传逻辑, 供 upload 和 retryCached 复用)
    static bool performUpload(const std::string& url,
                              const std::string& deviceId,
                              const std::string& timestamp,
                              const std::string& nonce,
                              const std::string& signature,
                              const std::string& hashB64,
                              const uint8_t* encryptedData,
                              size_t encryptedLen);

    /// 每轮主循环最多重试的条目数
    static const size_t MAX_RETRIES_PER_CYCLE = 2;

    /// 最大重试次数 (超过后丢弃)
    static const uint32_t MAX_RETRY_ATTEMPTS = 10;

    /// 缓存条目超时时间 (秒, 超过后丢弃)
    static const uint32_t RETRY_TIMEOUT_SECONDS = 3600;
};
