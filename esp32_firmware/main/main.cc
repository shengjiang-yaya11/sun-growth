/*
 * ============================================================
 *  【ESP32 端固件】v2.2 生产版 (DIO Flash, Quad PSRAM, FULL PHY)
 *  烧录目标: ESP32-S3 CAM V1.1 开发板
 *  作者: Andrew 亚生
 * ============================================================
 *
 * main.cc - 生物成长记录仪主程序
 *
 * ESP32-S3 CAM V1.1 + OV3660
 * C++ 核心 + mbedTLS 安全模块 (ADR-001)
 *
 * 工作流:
 *   1. 初始化 LED + BOOT 键
 *   2. 启动时 BOOT 键检测: 按住 BOOT 键上电 → 强制进入配网 (v2.2 新增)
 *   3. 初始化 NVS, 固件版本检测: 版本变更 → 自动清除旧配网 (v2.2 新增)
 *   4. 检查是否已配网
 *   5. 未配网: 进入 SoftAP Captive Portal 配网模式 (ADR-003)
 *   6. 已配网: 加载配置 -> 初始化 WiFi/摄像头/安全模块/重试队列
 *   7. 循环: 拍照 -> 加密签名 -> HTTP POST -> 等待间隔
 *   8. 失败重试: PSRAM 缓存 + 主循环重试 (ADR-007)
 *   9. 运行中工厂复位: 长按 BOOT 键 10 秒 (ADR-008)
 *  10. 堆监控: 定期检查内存状态 (ADR-010)
 *
 * v2.2 修复:
 *   - DIO Flash 模式 (硬件确认为DIO, 非QIO)
 *   - Quad PSRAM 模式 (Octal导致WiFi AP无法启动)
 *   - FULL PHY 射频校准 (工厂复位后恢复射频参数)
 *   - TX功率 20dBm (稳定模式)
 *   - 启动时 BOOT 键检测: 按住 BOOT 键上电 = 强制进入配网
 *   - 固件版本自动检测: 版本变更自动清除 NVS 旧配网数据
 *   - 修复工厂复位在启动阶段不生效的问题
 */

#include "config.h"
#include "camera_manager.h"
#include "wifi_manager.h"
#include "uploader.h"
#include "security_bridge.h"
#include "provisioning.h"
#include "retry_queue.h"

#include "esp_log.h"
#include "esp_sleep.h"
#include "esp_timer.h"
#include "esp_event.h"
#include "esp_netif.h"
#include "esp_netif_sntp.h"
#include "esp_heap_caps.h"
#include "esp_psram.h"
#include "freertos/FreeRTOS.h"
#include "freertos/task.h"
#include "nvs_flash.h"
#include "led_strip.h"
#include "driver/gpio.h"
#include <cstring>

static const char* TAG = "MAIN";

// RGB LED (WS2812, IO48)
static led_strip_handle_t s_led = nullptr;

// 状态计数 (RTC 内存, 深度睡眠保留)
RTC_DATA_ATTR uint32_t rtcPhotoCount = 0;
RTC_DATA_ATTR uint32_t rtcFailCount  = 0;

// 常驻模式计数
static uint32_t photoCount = 0;
static uint32_t failCount  = 0;

// 拍照间隔 (从 NVS 加载)
static uint32_t captureInterval = PROVISION_DEFAULT_INTERVAL;

// LED 状态指示
void setLED(uint8_t r, uint8_t g, uint8_t b) {
    if (!s_led) return;
    led_strip_set_pixel(s_led, 0, r, g, b);
    led_strip_refresh(s_led);
}

void ledBlue()   { setLED(0, 0, 32); }
void ledGreen()  { setLED(0, 32, 0); }
void ledYellow() { setLED(32, 24, 0); }
void ledRed()    { setLED(32, 0, 0); }
void ledOff()    { setLED(0, 0, 0); }

// 拍照并上传
bool captureAndUpload() {
    ledYellow();

    camera_fb_t* fb = CameraManager::capture();
    if (!fb) {
        ledRed();
        vTaskDelay(pdMS_TO_TICKS(500));
        return false;
    }

    bool ok = Uploader::upload(fb->buf, fb->len);
    CameraManager::returnFrame(fb);

    if (ok) {
        photoCount++;
        ESP_LOGI(TAG, "第 %u 张上传成功 (失败 %u, 待重试 %u)",
                 (unsigned)photoCount, (unsigned)failCount,
                 (unsigned)Uploader::getPendingRetryCount());
        ledGreen();
    } else {
        failCount++;
        ESP_LOGE(TAG, "上传失败 (累计 %u, 待重试 %u)",
                 (unsigned)failCount,
                 (unsigned)Uploader::getPendingRetryCount());
        ledRed();
        vTaskDelay(pdMS_TO_TICKS(1000));
    }

    return ok;
}

// 初始化 LED
void initLED() {
    led_strip_config_t ledConfig = {};
    ledConfig.strip_gpio_num = 48;
    ledConfig.max_leds = 1;
    ledConfig.led_model = LED_MODEL_WS2812;
    ledConfig.color_component_format = LED_STRIP_COLOR_COMPONENT_FMT_GRB;
    ledConfig.flags.invert_out = false;

    led_strip_rmt_config_t ledRmt = {};
    ledRmt.clk_src = RMT_CLK_SRC_DEFAULT;
    ledRmt.resolution_hz = 10 * 1000 * 1000;

    led_strip_new_rmt_device(&ledConfig, &ledRmt, &s_led);
    if (s_led) {
        led_strip_clear(s_led);
    }
}

// SNTP 时间同步 (用于签名时间戳)
void initSNTP() {
    setenv("TZ", "UTC0", 1);
    tzset();

    esp_sntp_config_t sntpCfg = ESP_NETIF_SNTP_DEFAULT_CONFIG("pool.ntp.org");
    esp_netif_sntp_init(&sntpCfg);
    esp_netif_sntp_start();

    // 等待同步 (最多 5 秒)
    int retry = 0;
    while (esp_netif_sntp_sync_wait(pdMS_TO_TICKS(1000)) != ESP_OK && retry < 5) {
        ESP_LOGI(TAG, "等待 NTP 同步... (%d)", retry + 1);
        retry++;
    }
    if (retry < 5) {
        time_t now;
        time(&now);
        ESP_LOGI(TAG, "NTP 同步成功: %lld", (long long)now);
    } else {
        ESP_LOGW(TAG, "NTP 同步超时, 使用 esp_timer 估算时间");
    }
}

// ============================================================
// ADR-008: 工厂复位 (长按 BOOT 键 10 秒)
// ============================================================

void initBootButton() {
    // BOOT 键接 GPIO0, 低电平有效
    gpio_config_t ioConf = {};
    ioConf.pin_bit_mask = (1ULL << BOOT_BUTTON_GPIO);
    ioConf.mode = GPIO_MODE_INPUT;
    ioConf.pull_up_en = GPIO_PULLUP_ENABLE;
    ioConf.pull_down_en = GPIO_PULLDOWN_DISABLE;
    ioConf.intr_type = GPIO_INTR_DISABLE;
    gpio_config(&ioConf);
    ESP_LOGI(TAG, "BOOT 键 (GPIO%d) 已初始化, 长按 %dms 恢复出厂",
             BOOT_BUTTON_GPIO, FACTORY_RESET_HOLD_MS);
}

bool checkFactoryReset() {
    // 检测 BOOT 键是否被按住
    if (gpio_get_level((gpio_num_t)BOOT_BUTTON_GPIO) != 0) {
        return false;  // 未按下
    }

    ESP_LOGW(TAG, "检测到 BOOT 键按下, 等待 %dms 确认工厂复位...",
             FACTORY_RESET_HOLD_MS);

    int heldMs = 0;
    while (gpio_get_level((gpio_num_t)BOOT_BUTTON_GPIO) == 0) {
        vTaskDelay(pdMS_TO_TICKS(BOOT_BUTTON_POLL_MS));
        heldMs += BOOT_BUTTON_POLL_MS;

        // 每 2 秒闪烁红色 LED 提示
        if (heldMs % 2000 == 0) {
            ESP_LOGW(TAG, "持续按住: %dms / %dms",
                     heldMs, FACTORY_RESET_HOLD_MS);
            ledRed();
            vTaskDelay(pdMS_TO_TICKS(50));
            ledOff();
        }

        if (heldMs >= FACTORY_RESET_HOLD_MS) {
            ESP_LOGW(TAG, "========================================");
            ESP_LOGW(TAG, "  工厂复位! 清除 NVS 并重启");
            ESP_LOGW(TAG, "========================================");

            // 红色常亮
            ledRed();

            // 擦除 NVS (清除配网信息)
            nvs_flash_erase();
            vTaskDelay(pdMS_TO_TICKS(500));

            ESP_LOGW(TAG, "NVS 已清除, 重启进入配网模式");
            esp_restart();
            return true;  // 永远不会执行到这里
        }
    }

    // 用户中途松开, 取消复位
    ESP_LOGI(TAG, "BOOT 键已释放, 取消工厂复位 (按住 %dms)", heldMs);
    return false;
}

// ============================================================
// ADR-010: 堆内存监控
// ============================================================

void logHeapStatus() {
    size_t minFreeHeap  = esp_get_minimum_free_heap_size();
    size_t freeInternal = heap_caps_get_free_size(MALLOC_CAP_INTERNAL);
    size_t freePsram    = heap_caps_get_free_size(MALLOC_CAP_SPIRAM);
    size_t largestBlock = heap_caps_get_largest_free_block(MALLOC_CAP_DEFAULT);

    ESP_LOGI(TAG, "堆内存: 内部=%u B (最小=%u B), PSRAM=%u B, 最大块=%u B",
             (unsigned)freeInternal, (unsigned)minFreeHeap,
             (unsigned)freePsram, (unsigned)largestBlock);

    // 低内存告警
    if (freeInternal < HEAP_LOW_THRESHOLD) {
        ESP_LOGE(TAG, "!!! 低内存告警 !!! 内部堆仅剩 %u B (阈值 %u B)",
                 (unsigned)freeInternal, (unsigned)HEAP_LOW_THRESHOLD);
        ESP_LOGE(TAG, "  最小空闲记录: %u B, PSRAM 可用: %u B",
                 (unsigned)minFreeHeap, (unsigned)freePsram);
        ESP_LOGE(TAG, "  重试队列占用: %u B, 待重试: %u 条",
                 (unsigned)RetryQueue::getUsedMemory(),
                 (unsigned)RetryQueue::size());
    }

    // 如果内部堆非常低, 清空重试队列释放 PSRAM 压力
    if (freeInternal < HEAP_LOW_THRESHOLD / 2 && !RetryQueue::empty()) {
        ESP_LOGE(TAG, "内部堆严重不足, 清空重试队列");
        RetryQueue::clear();
    }
}

// 外部入口
extern "C" void app_main() {
    ESP_LOGI(TAG, "========================================");
    ESP_LOGI(TAG, "  生物成长记录仪 (商用版) v%s", FIRMWARE_VERSION);
    ESP_LOGI(TAG, "  ESP32-S3 CAM + OV3660");
    ESP_LOGI(TAG, "  C++ core + mbedTLS security");
    ESP_LOGI(TAG, "  PSRAM retry + Factory reset + Auto-version-check");
    ESP_LOGI(TAG, "========================================");

    // 1. 初始化 LED
    initLED();
    ledBlue();

    // 2. 初始化 BOOT 键
    initBootButton();

    // 3. 启动时 BOOT 键检测 (v2.2 关键修复)
    //    按住 BOOT 键然后上电 → 强制进入配网模式
    //    解决: 固件更新后 NVS 中仍有旧配网数据, 导致跳过 SoftAP 的问题
    vTaskDelay(pdMS_TO_TICKS(200));  // 等待 GPIO 稳定
    if (gpio_get_level((gpio_num_t)BOOT_BUTTON_GPIO) == 0) {
        ESP_LOGW(TAG, "========================================");
        ESP_LOGW(TAG, "  检测到 BOOT 键在启动时被按住!");
        ESP_LOGW(TAG, "  强制进入配网模式 (清除 NVS)");
        ESP_LOGW(TAG, "========================================");
        ledYellow();
        // 先初始化 NVS (需要擦除前先 init)
        nvs_flash_init();
        nvs_flash_erase();
        ESP_LOGW(TAG, "NVS 已清除, 3 秒后重启进入配网模式");
        vTaskDelay(pdMS_TO_TICKS(3000));
        esp_restart();
        return;
    }

    // 4. 初始化 NVS
    esp_err_t ret = nvs_flash_init();
    if (ret == ESP_ERR_NVS_NO_FREE_PAGES || ret == ESP_ERR_NVS_NEW_VERSION_FOUND) {
        nvs_flash_erase();
        nvs_flash_init();
    }

    // 5. 固件版本自动检测 (v2.2 关键修复)
    //    如果 NVS 中存储的固件版本与当前编译版本不同,
    //    自动清除旧配网数据, 强制重新配网
    //    解决: 固件升级后, 旧配网数据与新版固件不兼容的问题
    {
        nvs_handle_t handle;
        bool needErase = false;
        if (nvs_open(NVS_NAMESPACE_PROVISION, NVS_READWRITE, &handle) == ESP_OK) {
            char storedVer[32] = {0};
            size_t verLen = sizeof(storedVer);
            if (nvs_get_str(handle, NVS_KEY_FW_VERSION, storedVer, &verLen) == ESP_OK) {
                if (strcmp(storedVer, FIRMWARE_VERSION) != 0) {
                    ESP_LOGW(TAG, "固件版本变更: %s → %s, 自动清除旧配网数据",
                             storedVer, FIRMWARE_VERSION);
                    needErase = true;
                }
            } else {
                // 旧固件没有存储版本号, 视为版本变更
                ESP_LOGW(TAG, "未检测到固件版本记录, 首次启动 v%s, 清除旧配网数据",
                         FIRMWARE_VERSION);
                needErase = true;
            }
            nvs_close(handle);
        }
        if (needErase) {
            nvs_flash_erase();
            nvs_flash_init();
            ESP_LOGI(TAG, "NVS 已清除, 将进入配网模式");
        }
    }

    // 6. 初始化网络接口和默认事件循环 (全局唯一)
    ESP_ERROR_CHECK(esp_netif_init());
    ESP_ERROR_CHECK(esp_event_loop_create_default());

    // 7. 检查是否已配网
    if (!Provisioning::isProvisioned()) {
        ESP_LOGI(TAG, "设备未配网, 进入 SoftAP Captive Portal 配网模式");
        ledYellow();
        Provisioning::start();  // 阻塞, 配网完成后自动重启
        return;  // 永远不会执行到这里
    }

    // 8. 从 NVS 加载配置
    Provisioning::Config cfg;
    if (!Provisioning::loadConfig(cfg)) {
        ESP_LOGE(TAG, "配置加载失败, 重新进入配网模式");
        Provisioning::start();
        return;
    }
    captureInterval = cfg.interval;

    // 9. 初始化安全模块 (从 NVS 加载密钥)
    SecurityBridge::init();

    // 10. 初始化摄像头
    if (!CameraManager::init()) {
        ESP_LOGE(TAG, "摄像头初始化失败, 10s 后重启");
        ledRed();
        vTaskDelay(pdMS_TO_TICKS(10000));
        esp_restart();
    }

    // 11. 初始化 WiFi
    WifiManager::init();

    // 12. 初始化 PSRAM 重试队列 (ADR-007)
    RetryQueue::init();

    // 13. 连接 WiFi
    if (!WifiManager::connect()) {
        ESP_LOGW(TAG, "WiFi 首次连接失败, 持续重试...");
    }

    // 14. NTP 时间同步
    initSNTP();

    // 15. 打印初始内存状态
    logHeapStatus();

    ESP_LOGI(TAG, "IP: %s", WifiManager::getIP().c_str());
    ledGreen();

    // ======================= 深度睡眠模式 =======================
    #if DEFAULT_DEEP_SLEEP
    ESP_LOGI(TAG, "省电模式 (深度睡眠), 间隔 %lu 秒", (unsigned long)captureInterval);
    ESP_LOGI(TAG, "已拍 %u, 失败 %u", (unsigned)rtcPhotoCount, (unsigned)rtcFailCount);

    photoCount = rtcPhotoCount;
    failCount  = rtcFailCount;

    WifiManager::ensureConnected();
    captureAndUpload();

    rtcPhotoCount = photoCount;
    rtcFailCount  = failCount;

    // 关闭外设省电
    CameraManager::deinit();
    WifiManager::disconnect();
    ledOff();

    ESP_LOGI(TAG, "进入深度睡眠 %lu 秒...", (unsigned long)captureInterval);
    esp_sleep_enable_timer_wakeup((uint64_t)captureInterval * 1000000ULL);
    esp_deep_sleep_start();
    #endif

    // ======================= 常驻模式 =======================
    ESP_LOGI(TAG, "常驻模式, 间隔 %lu 秒", (unsigned long)captureInterval);

    // 先处理重试队列中的遗留照片
    if (!RetryQueue::empty()) {
        ESP_LOGI(TAG, "发现 %u 条遗留缓存照片, 先重试",
                 (unsigned)RetryQueue::size());
        Uploader::processRetries();
    }

    // 立即拍第一张
    captureAndUpload();

    TickType_t lastCapture  = xTaskGetTickCount();
    TickType_t lastWifiCheck = 0;
    TickType_t lastHeapLog   = xTaskGetTickCount();
    TickType_t lastRetry     = xTaskGetTickCount();
    TickType_t interval      = pdMS_TO_TICKS(captureInterval * 1000);

    while (true) {
        TickType_t now = xTaskGetTickCount();

        // ADR-008: 检测工厂复位 (BOOT 键长按)
        if (gpio_get_level((gpio_num_t)BOOT_BUTTON_GPIO) == 0) {
            checkFactoryReset();
        }

        // 定时拍照
        if ((now - lastCapture) >= interval) {
            lastCapture = now;
            WifiManager::ensureConnected();
            captureAndUpload();
        }

        // ADR-007: 定期处理重试队列
        if ((now - lastRetry) >= pdMS_TO_TICKS(RETRY_INTERVAL_MS)) {
            lastRetry = now;
            if (!RetryQueue::empty()) {
                Uploader::processRetries();
            }
        }

        // 定期检查 WiFi
        if ((now - lastWifiCheck) >= pdMS_TO_TICKS(WIFI_RECONNECT_MS)) {
            lastWifiCheck = now;
            if (!WifiManager::isConnected()) {
                ledBlue();
                WifiManager::connect();
                if (WifiManager::isConnected()) ledGreen();
            }
        }

        // ADR-010: 定期堆内存监控
        if ((now - lastHeapLog) >= pdMS_TO_TICKS(HEAP_MONITOR_INTERVAL_MS)) {
            lastHeapLog = now;
            logHeapStatus();
        }

        vTaskDelay(pdMS_TO_TICKS(100));
    }
}
