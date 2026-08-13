/*
 * ============================================================
 *  command_server.cc - TCP 命令接收服务器实现 (v3.1)
 * ============================================================
 */

#include "command_server.h"
#include "config.h"
#include "security_bridge.h"
#include "esp_log.h"
#include "esp_wifi.h"
#include "esp_system.h"
#include "esp_timer.h"
#include "freertos/FreeRTOS.h"
#include "freertos/task.h"
#include "lwip/sockets.h"
#include "lwip/inet.h"
#include "lwip/netdb.h"
#include "nvs_flash.h"

#include <cstring>
#include <cstdio>
#include <cstdlib>
#include <ctime>
#include <map>
#include <mutex>

static const char* TAG = "CMD_SRV";

// 全局状态
static uint16_t g_port = 8081;
static volatile bool g_running = false;
static std::map<std::string, CommandHandler> g_handlers;
static std::mutex g_mutex;

// 默认命令处理
namespace {

std::string handleCapture(const std::string& params) {
    // 需要在 main.cc 中注册的拍照回调
    return "{\"status\":\"ok\",\"action\":\"capture_queued\"}";
}

std::string handleStatus(const std::string& params) {
    char buf[512];
    esp_netif_ip_info_t ipInfo;
    std::string ip = "unknown";
    esp_netif_t* netif = esp_netif_get_handle_from_ifkey("WIFI_STA_DEF");
    if (netif && esp_netif_get_ip_info(netif, &ipInfo) == ESP_OK) {
        snprintf(buf, sizeof(buf), IPSTR, IP2STR(&ipInfo.ip));
        ip = buf;
    }

    int64_t uptime = esp_timer_get_time() / 1000000;
    size_t freeHeap = esp_get_free_heap_size();
    int rssi = 0;
    wifi_ap_record_t apInfo;
    if (esp_wifi_sta_get_ap_info(&apInfo) == ESP_OK) {
        rssi = apInfo.rssi;
    }

    snprintf(buf, sizeof(buf),
        "{\"status\":\"ok\",\"data\":{"
        "\"ip\":\"%s\","
        "\"uptime_sec\":%lld,"
        "\"free_heap\":%u,"
        "\"wifi_rssi\":%d,"
        "\"fw_version\":\"%s\""
        "}}",
        ip.c_str(), uptime, (unsigned)freeHeap, rssi, FIRMWARE_VERSION);
    return std::string(buf);
}

std::string handleSetConfig(const std::string& params) {
    // 解析 {"interval": 60} 并写入 NVS
    // 简化实现: 提取 interval 字段
    int interval = 0;
    const char* p = params.c_str();
    const char* pos = strstr(p, "\"interval\"");
    if (pos) {
        pos = strchr(pos, ':');
        if (pos) interval = atoi(pos + 1);
    }
    if (interval < 5 || interval > 86400) {
        return "{\"status\":\"error\",\"msg\":\"invalid interval (5-86400)\"}";
    }

    nvs_handle_t handle;
    if (nvs_open(NVS_NAMESPACE_PROVISION, NVS_READWRITE, &handle) == ESP_OK) {
        nvs_set_u32(handle, NVS_KEY_INTERVAL, (uint32_t)interval);
        nvs_commit(handle);
        nvs_close(handle);
    }
    char buf[128];
    snprintf(buf, sizeof(buf), "{\"status\":\"ok\",\"interval\":%d}", interval);
    return std::string(buf);
}

std::string handleRestart(const std::string& params) {
    ESP_LOGW(TAG, "收到重启命令, 3 秒后重启...");
    vTaskDelay(pdMS_TO_TICKS(3000));
    esp_restart();
    return "{\"status\":\"ok\"}";
}

} // anonymous namespace

// ==================== 签名验证 ====================

namespace {

bool extractJsonString(const std::string& json, const char* key, std::string& out) {
    std::string needle = std::string("\"") + key + "\"";
    size_t keyPos = json.find(needle);
    if (keyPos == std::string::npos) return false;

    size_t colon = json.find(':', keyPos + needle.size());
    if (colon == std::string::npos) return false;

    size_t open = json.find('"', colon + 1);
    if (open == std::string::npos) return false;
    size_t close = json.find('"', open + 1);
    if (close == std::string::npos) return false;
    out = json.substr(open + 1, close - open - 1);
    return true;
}

bool extractJsonObject(const std::string& json, const char* key, std::string& out) {
    std::string needle = std::string("\"") + key + "\"";
    size_t keyPos = json.find(needle);
    if (keyPos == std::string::npos) return false;

    size_t colon = json.find(':', keyPos + needle.size());
    if (colon == std::string::npos) return false;

    size_t open = json.find('{', colon + 1);
    if (open == std::string::npos) return false;

    int depth = 1;
    size_t pos = open + 1;
    while (pos < json.size() && depth > 0) {
        if (json[pos] == '{') depth++;
        else if (json[pos] == '}') depth--;
        pos++;
    }
    if (depth != 0) return false;
    out = json.substr(open, pos - open);
    return true;
}

bool extractJsonInt64(const std::string& json, const char* key, int64_t& out) {
    std::string needle = std::string("\"") + key + "\"";
    size_t keyPos = json.find(needle);
    if (keyPos == std::string::npos) return false;

    size_t colon = json.find(':', keyPos + needle.size());
    if (colon == std::string::npos) return false;

    size_t pos = colon + 1;
    while (pos < json.size() && (json[pos] == ' ' || json[pos] == '"')) pos++;
    char* end = nullptr;
    out = (int64_t)strtoll(json.c_str() + pos, &end, 10);
    return end != json.c_str() + pos;
}

} // namespace

bool CommandServer::verifySignature(const std::string& cmd,
                                    const std::string& params,
                                    const std::string& body) {
    // PC 端签名算法: signing_key = HMAC(secret, "bio-cmd-sig-v1")
    //               sig = HMAC(signing_key, cmd || params_json || ts)
    std::string sigB64;
    int64_t ts = 0;
    if (!extractJsonString(body, "sig", sigB64) ||
        !extractJsonInt64(body, "ts", ts)) {
        ESP_LOGW(TAG, "命令缺少签名或时间戳");
        return false;
    }

    // 防重放: 时间戳在 5 分钟内
    int64_t now = (int64_t)time(nullptr);
    if (now < 1700000000) now = 1700000000 + (esp_timer_get_time() / 1000000);
    if (now - ts > 300 || ts - now > 300) {
        ESP_LOGW(TAG, "命令时间戳超出容差");
        return false;
    }

    std::string signPayload = cmd + params + std::to_string(ts);
    std::string expected = SecurityBridge::commandSignatureBase64(signPayload);
    if (expected.empty() || expected != sigB64) {
        ESP_LOGW(TAG, "命令签名验证失败");
        return false;
    }
    return true;
}

// ==================== 命令处理 ====================

std::string CommandServer::processCommand(const std::string& requestJson) {
    // 提取 cmd 字段
    std::string cmd;
    if (!extractJsonString(requestJson, "cmd", cmd)) {
        return buildResponse("error", "{\"msg\":\"missing cmd field\"}");
    }

    // 提取 params 字段 (缺省按空对象处理，与电脑端签名内容一致)
    std::string params = "{}";
    extractJsonObject(requestJson, "params", params);

    // status 仅用于局域网设备发现和状态查看，允许不签名；
    // 其它控制命令必须通过 HMAC 签名验证。
    if (cmd != "status") {
        if (!verifySignature(cmd, params, requestJson)) {
            return buildResponse("error", "{\"msg\":\"signature verification failed\"}");
        }
    }

    // 查找处理器
    std::lock_guard<std::mutex> lock(g_mutex);
    auto it = g_handlers.find(cmd);
    if (it != g_handlers.end()) {
        return it->second(params);
    }

    // 内置命令
    if (cmd == "status")     return handleStatus(params);
    if (cmd == "capture")    return handleCapture(params);
    if (cmd == "set_config") return handleSetConfig(params);
    if (cmd == "restart")    return handleRestart(params);

    return buildResponse("error", "{\"msg\":\"unknown command: " + cmd + "\"}");
}

// ==================== TCP 服务器 ====================

void CommandServer::serverTask(void* arg) {
    uint16_t port = *(uint16_t*)arg;
    delete (uint16_t*)arg;

    int listenSock = socket(AF_INET, SOCK_STREAM, 0);
    if (listenSock < 0) {
        ESP_LOGE(TAG, "创建 socket 失败");
        g_running = false;
        vTaskDelete(nullptr);
        return;
    }

    int opt = 1;
    setsockopt(listenSock, SOL_SOCKET, SO_REUSEADDR, &opt, sizeof(opt));

    struct sockaddr_in addr;
    memset(&addr, 0, sizeof(addr));
    addr.sin_family = AF_INET;
    addr.sin_addr.s_addr = htonl(INADDR_ANY);
    addr.sin_port = htons(port);

    if (bind(listenSock, (struct sockaddr*)&addr, sizeof(addr)) < 0) {
        ESP_LOGE(TAG, "绑定端口 %d 失败", port);
        close(listenSock);
        g_running = false;
        vTaskDelete(nullptr);
        return;
    }

    if (listen(listenSock, 3) < 0) {
        ESP_LOGE(TAG, "监听失败");
        close(listenSock);
        g_running = false;
        vTaskDelete(nullptr);
        return;
    }

    ESP_LOGI(TAG, "命令服务器已启动 (端口 %d), 等待 PC 连接...", port);

    while (g_running) {
        struct sockaddr_in clientAddr;
        socklen_t clientLen = sizeof(clientAddr);
        int clientSock = accept(listenSock, (struct sockaddr*)&clientAddr, &clientLen);

        if (clientSock < 0) {
            if (g_running) ESP_LOGW(TAG, "accept 失败");
            continue;
        }

        // 设置接收超时 5 秒
        struct timeval tv = {5, 0};
        setsockopt(clientSock, SOL_SOCKET, SO_RCVTIMEO, &tv, sizeof(tv));

        char buf[2048] = {0};
        int len = recv(clientSock, buf, sizeof(buf) - 1, 0);

        if (len > 0) {
            buf[len] = '\0';
            ESP_LOGI(TAG, "收到命令 (%d bytes): %.100s", len, buf);

            std::string response = processCommand(std::string(buf));
            send(clientSock, response.c_str(), response.size(), 0);
        }

        close(clientSock);
    }

    close(listenSock);
    ESP_LOGI(TAG, "命令服务器已停止");
    g_running = false;
    vTaskDelete(nullptr);
}

void CommandServer::start(uint16_t port) {
    if (g_running) {
        ESP_LOGW(TAG, "命令服务器已在运行");
        return;
    }
    g_port = port;
    g_running = true;

    uint16_t* portArg = new uint16_t(port);
    xTaskCreate(serverTask, "cmd_server", 6144, portArg, 5, nullptr);
    ESP_LOGI(TAG, "命令服务器任务已创建 (端口 %d)", port);
}

void CommandServer::stop() {
    g_running = false;
}

bool CommandServer::isRunning() {
    return g_running;
}

void CommandServer::registerHandler(const std::string& cmd, CommandHandler handler) {
    std::lock_guard<std::mutex> lock(g_mutex);
    g_handlers[cmd] = handler;
}

uint16_t CommandServer::getPort() {
    return g_port;
}
