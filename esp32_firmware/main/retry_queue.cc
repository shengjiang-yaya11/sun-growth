/*
 * ============================================================
 *  【ESP32 端固件】烧录目标: ESP32-S3 CAM V1.1 开发板
 *  编译工具链: ESP-IDF v5.x + Xtensa gcc (不依赖电脑端软件)
 *  烧录方式: USB 连接开发板, 运行 idf.py flash
 * ============================================================
 *
 * retry_queue.cc - PSRAM 照片缓存重试队列实现
 *
 * ADR-007: 上传失败重试 + PSRAM 照片缓存
 *
 * 环形缓冲区实现, 使用 std::string 自动管理 PSRAM 内存。
 * 队列满时丢弃最旧条目, 保证最新照片优先缓存。
 */

#include "retry_queue.h"
#include "esp_log.h"
#include "esp_heap_caps.h"
#include "esp_psram.h"
#include "esp_timer.h"
#include <cstring>

static const char* TAG = "RETRY";

// 静态成员初始化
RetryQueue::Entry  RetryQueue::entries_[MAX_RETRY_ENTRIES];
size_t RetryQueue::head_  = 0;
size_t RetryQueue::tail_  = 0;
size_t RetryQueue::count_ = 0;
bool   RetryQueue::initialized_ = false;

void RetryQueue::init() {
    head_ = 0;
    tail_ = 0;
    count_ = 0;
    initialized_ = true;

    size_t psramSize = esp_psram_get_size();
    size_t freePsram = heap_caps_get_free_size(MALLOC_CAP_SPIRAM);

    ESP_LOGI(TAG, "PSRAM 重试队列已初始化 (最大 %d 条目)", MAX_RETRY_ENTRIES);
    ESP_LOGI(TAG, "PSRAM 总量: %u bytes, 可用: %u bytes",
             (unsigned)psramSize, (unsigned)freePsram);
}

bool RetryQueue::push(const Entry& entry) {
    if (!initialized_) {
        ESP_LOGE(TAG, "队列未初始化");
        return false;
    }

    // 检查 PSRAM 可用空间 (至少需要 plaintextData 大小 + 1KB 余量)
    size_t needed = entry.plaintextData.size() + 1024;
    size_t freePsram = heap_caps_get_free_size(MALLOC_CAP_SPIRAM);

    if (freePsram < needed) {
        ESP_LOGW(TAG, "PSRAM 不足 (需要 %u, 可用 %u), 丢弃最旧条目",
                 (unsigned)needed, (unsigned)freePsram);
        // 丢弃最旧条目腾出空间
        if (count_ > 0) {
            entries_[head_].plaintextData.clear();
            entries_[head_].plaintextData.shrink_to_fit();
            head_ = (head_ + 1) % MAX_RETRY_ENTRIES;
            count_--;
        }
        // 再次检查
        freePsram = heap_caps_get_free_size(MALLOC_CAP_SPIRAM);
        if (freePsram < needed) {
            ESP_LOGE(TAG, "PSRAM 仍然不足, 放弃缓存");
            return false;
        }
    }

    // 队列满时丢弃最旧条目
    if (count_ >= MAX_RETRY_ENTRIES) {
        ESP_LOGW(TAG, "队列已满, 丢弃最旧条目");
        entries_[head_].plaintextData.clear();
        entries_[head_].plaintextData.shrink_to_fit();
        head_ = (head_ + 1) % MAX_RETRY_ENTRIES;
        count_--;
    }

    // 入队
    entries_[tail_] = entry;
    // 记录创建时间 (仅首次入队时设置, 重试失败重入队时保留原始时间用于超时判定)
    if (entries_[tail_].createdAt == 0) {
        entries_[tail_].createdAt = (uint32_t)(esp_timer_get_time() / 1000000);
    }
    tail_ = (tail_ + 1) % MAX_RETRY_ENTRIES;
    count_++;

    ESP_LOGI(TAG, "照片已缓存到重试队列 (队列: %d/%d, 原始大小: %u B)",
             (int)count_, (int)MAX_RETRY_ENTRIES, (unsigned)entry.plaintextData.size());

    return true;
}

bool RetryQueue::pop(Entry& entry) {
    if (!initialized_ || count_ == 0) {
        return false;
    }

    entry = entries_[head_];
    entries_[head_].plaintextData.clear();
    entries_[head_].plaintextData.shrink_to_fit();
    head_ = (head_ + 1) % MAX_RETRY_ENTRIES;
    count_--;

    return true;
}

bool RetryQueue::peek(Entry& entry) {
    if (!initialized_ || count_ == 0) {
        return false;
    }

    entry = entries_[head_];
    return true;
}

size_t RetryQueue::size() {
    return count_;
}

bool RetryQueue::empty() {
    return count_ == 0;
}

bool RetryQueue::full() {
    return count_ >= MAX_RETRY_ENTRIES;
}

void RetryQueue::clear() {
    for (size_t i = 0; i < MAX_RETRY_ENTRIES; i++) {
        entries_[i].plaintextData.clear();
        entries_[i].plaintextData.shrink_to_fit();
    }
    head_ = 0;
    tail_ = 0;
    count_ = 0;
    ESP_LOGI(TAG, "重试队列已清空");
}

size_t RetryQueue::getUsedMemory() {
    size_t total = 0;
    for (size_t i = 0; i < MAX_RETRY_ENTRIES; i++) {
        total += entries_[i].plaintextData.size();
        total += entries_[i].deviceId.size();
    }
    return total;
}
