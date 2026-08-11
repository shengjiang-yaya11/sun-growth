/*
 * ============================================================
 *  【ESP32 端固件】烧录目标: ESP32-S3 CAM V1.1 开发板
 *  编译工具链: ESP-IDF v5.x + Xtensa gcc (不依赖电脑端软件)
 *  烧录方式: USB 连接开发板, 运行 idf.py flash
 * ============================================================
 *
 * wifi_manager.cc - WiFi 连接管理实现
 */

#include "wifi_manager.h"
#include "config.h"
#include "esp_wifi.h"
#include "esp_event.h"
#include "esp_log.h"
#include "nvs_flash.h"
#include "freertos/FreeRTOS.h"
#include "freertos/event_groups.h"
#include <cstring>

static const char* TAG = "WIFI";
bool WifiManager::initialized_ = false;
bool WifiManager::connected_ = false;

static EventGroupHandle_t s_wifiEventGroup;
static const int WIFI_CONNECTED_BIT = BIT0;
static const int WIFI_FAIL_BIT      = BIT1;
static int s_retryCount = 0;

static void wifiEventHandler(void* arg, esp_event_base_t base,
                             int32_t id, void* data) {
    if (base == WIFI_EVENT && id == WIFI_EVENT_STA_START) {
        esp_wifi_connect();
    } else if (base == WIFI_EVENT && id == WIFI_EVENT_STA_DISCONNECTED) {
        WifiManager::setConnected(false);
        if (s_retryCount < 5) {
            esp_wifi_connect();
            s_retryCount++;
            ESP_LOGW(TAG, "重连中... (%d)", s_retryCount);
        } else {
            xEventGroupSetBits(s_wifiEventGroup, WIFI_FAIL_BIT);
        }
    } else if (base == IP_EVENT && id == IP_EVENT_STA_GOT_IP) {
        ip_event_got_ip_t* event = (ip_event_got_ip_t*)data;
        ESP_LOGI(TAG, "获取 IP: " IPSTR, IP2STR(&event->ip_info.ip));
        s_retryCount = 0;
        WifiManager::setConnected(true);
        xEventGroupSetBits(s_wifiEventGroup, WIFI_CONNECTED_BIT);
    }
}

bool WifiManager::init() {
    if (initialized_) return true;

    // NVS 和网络接口已在 main.cc 中初始化, 此处不再重复调用
    // (重复调用 esp_netif_init() 会触发 ESP_ERROR_CHECK 中断)

    s_wifiEventGroup = xEventGroupCreate();

    // 创建 STA netif (事件循环已在 main.cc 创建)
    esp_netif_create_default_wifi_sta();

    wifi_init_config_t cfg = WIFI_INIT_CONFIG_DEFAULT();
    ESP_ERROR_CHECK(esp_wifi_init(&cfg));

    esp_event_handler_instance_t anyId, gotIp;
    esp_event_handler_instance_register(WIFI_EVENT, ESP_EVENT_ANY_ID,
                                        &wifiEventHandler, nullptr, &anyId);
    esp_event_handler_instance_register(IP_EVENT, IP_EVENT_STA_GOT_IP,
                                        &wifiEventHandler, nullptr, &gotIp);

    initialized_ = true;
    ESP_LOGI(TAG, "WiFi 子系统初始化完成");
    return true;
}

bool WifiManager::connect() {
    if (connected_) return true;

    // 从 NVS "provision" 命名空间读取 WiFi 配置
    std::string ssid;
    std::string pass;

    nvs_handle_t handle;
    if (nvs_open(NVS_NAMESPACE_PROVISION, NVS_READONLY, &handle) == ESP_OK) {
        char buf[64] = {0};
        size_t len = sizeof(buf);
        if (nvs_get_str(handle, NVS_KEY_SSID, buf, &len) == ESP_OK) ssid = buf;
        len = sizeof(buf);
        if (nvs_get_str(handle, NVS_KEY_PASS, buf, &len) == ESP_OK) pass = buf;
        nvs_close(handle);
    }

    if (ssid.empty()) {
        ESP_LOGE(TAG, "NVS 中未找到 WiFi 配置, 请先配网");
        return false;
    }

    // 清除上一次连接尝试遗留的事件位 (防止立即返回 FAIL)
    xEventGroupClearBits(s_wifiEventGroup, WIFI_CONNECTED_BIT | WIFI_FAIL_BIT);
    // 重置重试计数器 (上次失败后 s_retryCount 可能已达到上限)
    s_retryCount = 0;

    wifi_config_t wifiCfg = {};
    strncpy((char*)wifiCfg.sta.ssid, ssid.c_str(), sizeof(wifiCfg.sta.ssid) - 1);
    strncpy((char*)wifiCfg.sta.password, pass.c_str(), sizeof(wifiCfg.sta.password) - 1);

    ESP_ERROR_CHECK(esp_wifi_set_mode(WIFI_MODE_STA));
    ESP_ERROR_CHECK(esp_wifi_set_config(WIFI_IF_STA, &wifiCfg));

    // 启动 WiFi 驱动 (首次调用触发 WIFI_EVENT_STA_START, 事件处理器自动 esp_wifi_connect)
    // 已启动时返回 ESP_ERR_WIFI_STATE, 需手动触发连接
    esp_err_t startErr = esp_wifi_start();
    if (startErr == ESP_ERR_WIFI_STATE || startErr == ESP_ERR_INVALID_STATE) {
        esp_wifi_connect();
    } else if (startErr != ESP_OK) {
        ESP_LOGE(TAG, "esp_wifi_start 失败: %s", esp_err_to_name(startErr));
        return false;
    }

    ESP_LOGI(TAG, "连接 WiFi: %s", ssid.c_str());

    // 等待连接 (最多 20 秒)
    EventBits_t bits = xEventGroupWaitBits(
        s_wifiEventGroup,
        WIFI_CONNECTED_BIT | WIFI_FAIL_BIT,
        pdFALSE, pdFALSE,
        pdMS_TO_TICKS(20000)
    );

    if (bits & WIFI_CONNECTED_BIT) {
        ESP_LOGI(TAG, "WiFi 已连接");
        return true;
    }

    ESP_LOGE(TAG, "WiFi 连接失败");
    return false;
}

void WifiManager::ensureConnected() {
    if (!connected_) {
        ESP_LOGI(TAG, "WiFi 断开, 重连...");
        connect();
    }
}

bool WifiManager::isConnected() {
    return connected_;
}

std::string WifiManager::getIP() {
    esp_netif_ip_info_t ipInfo;
    esp_netif_t* netif = esp_netif_get_handle_from_ifkey("WIFI_STA_DEF");
    if (netif && esp_netif_get_ip_info(netif, &ipInfo) == ESP_OK) {
        char buf[16];
        snprintf(buf, sizeof(buf), IPSTR, IP2STR(&ipInfo.ip));
        return buf;
    }
    return "0.0.0.0";
}

void WifiManager::disconnect() {
    if (connected_) {
        esp_wifi_disconnect();
        esp_wifi_stop();
        connected_ = false;
        ESP_LOGI(TAG, "WiFi 已断开");
    }
}
