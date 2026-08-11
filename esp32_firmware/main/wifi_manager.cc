/*
 * wifi_manager.cc - WiFi 连接管理 (v3.1 指数退避版)
 */

#include "wifi_manager.h"
#include "config.h"
#include "esp_wifi.h"
#include "esp_event.h"
#include "esp_netif.h"
#include "esp_log.h"
#include "nvs_flash.h"
#include "freertos/FreeRTOS.h"
#include "freertos/event_groups.h"
#include "freertos/timers.h"
#include <cstring>

static const char* TAG = "WIFI";
bool WifiManager::initialized_ = false;
bool WifiManager::connected_ = false;
bool WifiManager::connecting_ = false;

static EventGroupHandle_t s_wifiEventGroup;
static const int WIFI_CONNECTED_BIT = BIT0;
static const int WIFI_FAIL_BIT      = BIT1;
static int s_retryCount = 0;
static TimerHandle_t s_reconnectTimer = nullptr;

static void reconnectTimerCB(TimerHandle_t) {
    WifiManager::setConnecting(false);
    WifiManager::connect();
}

static void wifiEventHandler(void* arg, esp_event_base_t base,
                             int32_t id, void* data) {
    if (base == WIFI_EVENT && id == WIFI_EVENT_STA_START) {
        esp_wifi_connect();
    } else if (base == WIFI_EVENT && id == WIFI_EVENT_STA_DISCONNECTED) {
        wifi_event_sta_disconnected_t* ev = (wifi_event_sta_disconnected_t*)data;
        WifiManager::setConnected(false);
        WifiManager::setConnecting(false);
        ESP_LOGW(TAG, "WiFi 断开 (reason=%d), 重试 #%d", ev->reason, s_retryCount + 1);
        
        // 指数退避: 1s, 2s, 4s, 8s, 16s, 32s, 60s (max)
        uint32_t delay = (1u << s_retryCount) * 1000;
        if (delay > 60000) delay = 60000;
        s_retryCount++;
        
        ESP_LOGI(TAG, "%d ms 后重连...", (int)delay);
        if (!s_reconnectTimer) {
            s_reconnectTimer = xTimerCreate("wifiRetry",
                pdMS_TO_TICKS(delay), pdFALSE, nullptr, reconnectTimerCB);
        } else {
            xTimerChangePeriod(s_reconnectTimer, pdMS_TO_TICKS(delay), 0);
        }
        xTimerStart(s_reconnectTimer, 0);
    } else if (base == IP_EVENT && id == IP_EVENT_STA_GOT_IP) {
        ip_event_got_ip_t* event = (ip_event_got_ip_t*)data;
        ESP_LOGI(TAG, "获取 IP: " IPSTR, IP2STR(&event->ip_info.ip));
        s_retryCount = 0;
        WifiManager::setConnecting(false);
        WifiManager::setConnected(true);
        xEventGroupSetBits(s_wifiEventGroup, WIFI_CONNECTED_BIT);
        if (s_reconnectTimer) xTimerStop(s_reconnectTimer, 0);
    }
}

bool WifiManager::init() {
    if (initialized_) return true;

    s_wifiEventGroup = xEventGroupCreate();
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
    if (WifiManager::connecting_) return false;  // 防止重入
    
    connecting_ = true;

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
        ESP_LOGE(TAG, "NVS 中未找到 WiFi 配置");
        connecting_ = false;
        return false;
    }

    xEventGroupClearBits(s_wifiEventGroup, WIFI_CONNECTED_BIT | WIFI_FAIL_BIT);

    wifi_config_t wifiCfg = {};
    strncpy((char*)wifiCfg.sta.ssid, ssid.c_str(), sizeof(wifiCfg.sta.ssid) - 1);
    strncpy((char*)wifiCfg.sta.password, pass.c_str(), sizeof(wifiCfg.sta.password) - 1);

    ESP_ERROR_CHECK(esp_wifi_set_mode(WIFI_MODE_STA));
    ESP_ERROR_CHECK(esp_wifi_set_config(WIFI_IF_STA, &wifiCfg));

    esp_err_t startErr = esp_wifi_start();
    if (startErr == ESP_ERR_WIFI_STATE || startErr == ESP_ERR_INVALID_STATE) {
        esp_wifi_connect();
    } else if (startErr != ESP_OK) {
        ESP_LOGE(TAG, "esp_wifi_start 失败: %s", esp_err_to_name(startErr));
        connecting_ = false;
        return false;
    }

    ESP_LOGI(TAG, "连接 WiFi: %s", ssid.c_str());

    EventBits_t bits = xEventGroupWaitBits(
        s_wifiEventGroup, WIFI_CONNECTED_BIT | WIFI_FAIL_BIT,
        pdFALSE, pdFALSE, pdMS_TO_TICKS(20000));

    connecting_ = false;
    if (bits & WIFI_CONNECTED_BIT) {
        ESP_LOGI(TAG, "WiFi 已连接");
        return true;
    }

    ESP_LOGE(TAG, "WiFi 连接失败");
    return false;
}

void WifiManager::ensureConnected() {
    if (!connected_ && !connecting_) {
        ESP_LOGI(TAG, "WiFi 断开, 重连...");
        connect();
    }
}

bool WifiManager::isConnected() { return connected_; }


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
