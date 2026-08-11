/*
 * ============================================================
 *  【ESP32 端固件】烧录目标: ESP32-S3 CAM V1.1 开发板
 *  编译工具链: ESP-IDF v5.x + Xtensa gcc (不依赖电脑端软件)
 *  烧录方式: USB 连接开发板, 运行 idf.py flash
 * ============================================================
 *
 * retry_queue.h - PSRAM 照片缓存重试队列
 *
 * ADR-007: 上传失败重试 + PSRAM 照片缓存
 *
 * 网络波动导致上传失败时, 将加密后的照片数据缓存到 PSRAM 环形缓冲区,
 * 主循环中定期重试, 保证数据可靠性。
 *
 * 设计:
 *   - 环形缓冲区, 最多缓存 MAX_RETRY_ENTRIES 张照片
 *   - 每张照片包含: 加密数据 + 元数据 (时间戳/nonce/hash 等)
 *   - PSRAM 分配, 不占用内部 SRAM
 *   - 队列满时丢弃最旧条目 (先进先出)
 */

#pragma once

#include <stdint.h>
#include <stddef.h>
#include <string>

class RetryQueue {
public:
    /*
     * 单个缓存条目
     * 存储原始明文, 重试时重新加密/签名 (防止 nonce 重放和时间戳过期)
     */
    struct Entry {
        std::string plaintextData;   // 原始 JPEG 明文 (用于重新加密)
        std::string deviceId;        // 设备 ID
        uint32_t    originalLen;     // 原始 JPEG 长度
        uint32_t    retryCount;      // 重试次数
        uint32_t    createdAt;       // 创建时间 (秒, 用于超时丢弃)

        Entry() : originalLen(0), retryCount(0), createdAt(0) {}
    };

    /// 初始化队列 (分配 PSRAM 内存)
    static void init();

    /// 入队一个失败的上传条目
    /// 返回: true = 成功入队, false = 队列满 (已丢弃最旧条目)
    static bool push(const Entry& entry);

    /// 出队一个条目用于重试
    /// 返回: true = 成功取出, false = 队列为空
    static bool pop(Entry& entry);

    /// 查看队首条目 (不出队)
    static bool peek(Entry& entry);

    /// 当前队列中的条目数
    static size_t size();

    /// 队列是否为空
    static bool empty();

    /// 队列是否已满
    static bool full();

    /// 清空队列 (释放所有 PSRAM 内存)
    static void clear();

    /// 获取 PSRAM 使用情况 (字节)
    static size_t getUsedMemory();

private:
    static const size_t MAX_RETRY_ENTRIES = 8;  // 最多缓存 8 张照片
    static Entry  entries_[MAX_RETRY_ENTRIES];
    static size_t head_;  // 出队位置
    static size_t tail_;  // 入队位置
    static size_t count_; // 当前条目数
    static bool   initialized_;
};
