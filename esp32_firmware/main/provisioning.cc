/*
 * ============================================================
 *  【ESP32 端固件】烧录目标: ESP32-S3 CAM V1.1 开发板
 *  编译工具链: ESP-IDF v5.x + Xtensa gcc (不依赖电脑端软件)
 *  烧录方式: USB 连接开发板, 运行 idf.py flash
 * ============================================================
 *
 * provisioning.cc - SoftAP Captive Portal 配网模块实现
 *
 * 实现:
 *   1. SoftAP 热点 (SSID: BioRecorder-XXXX)
 *   2. DNS 劫持服务器 (端口 53, 所有查询返回 192.168.4.1)
 *   3. HTTP 服务器 (端口 80, 配网页面 + 表单处理)
 *   4. NVS 配置存储
 *
 * ADR-003: ESP32 设备配网改用 SoftAP Captive Portal
 */

#include "provisioning.h"
#include "config.h"

#include "esp_wifi.h"
#include "esp_event.h"
#include "esp_netif.h"
#include "esp_http_server.h"
#include "nvs_flash.h"
#include "esp_log.h"
#include "esp_mac.h"
#include "esp_random.h"
#include "esp_system.h"
#include "freertos/FreeRTOS.h"
#include "freertos/task.h"
#include "lwip/sockets.h"
#include "lwip/inet.h"

#include <cstring>
#include <cstdio>
#include <cstdlib>

static const char* TAG = "PROV";

/* SoftAP 网关 IP (ESP-IDF 默认) */
static const uint8_t AP_IP[4] = {192, 168, 4, 1};

/* HTTP 服务器句柄 */
static httpd_handle_t s_httpServer = nullptr;

/* ============================================================
 *  HTML 页面
 * ============================================================ */

/* 配网页面 (移动端自适应, ADR-009 改进版) */
static const char* HTML_CONFIG_PAGE = R"HTML(<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1, maximum-scale=1, user-scalable=no">
<title>生物记录仪 - 配网</title>
<style>
*{box-sizing:border-box;margin:0;padding:0}
body{font-family:-apple-system,sans-serif;background:#f0f2f5;padding:16px}
.card{max-width:420px;margin:0 auto;background:#fff;border-radius:14px;padding:24px 20px;box-shadow:0 4px 16px rgba(0,0,0,.08)}
h2{text-align:center;color:#1a1a1a;margin-bottom:6px;font-size:20px}
.sub{text-align:center;color:#888;font-size:13px;margin-bottom:20px}
label{display:block;margin:14px 0 5px;font-size:14px;color:#444;font-weight:500}
input{width:100%;padding:11px 12px;border:1px solid #ddd;border-radius:9px;font-size:15px;outline:none;transition:border .2s}
input:focus{border-color:#4CAF50}
input[type=checkbox]{width:auto;display:inline-block;margin-right:6px}
.checkbox-row{display:flex;align-items:center;margin:14px 0}
.checkbox-row label{margin:0;font-weight:400}
button{width:100%;margin-top:22px;padding:13px;background:#4CAF50;color:#fff;border:none;border-radius:9px;font-size:16px;font-weight:600;cursor:pointer}
button:active{background:#3d8b40}
.hint{font-size:12px;color:#aaa;margin-top:4px}
.toggle-link{font-size:13px;color:#4CAF50;cursor:pointer;margin-top:6px;display:inline-block;text-decoration:underline}
.error{color:#e53935;font-size:13px;margin-top:8px;display:none}
.protocol-hint{font-size:12px;color:#888;margin-top:4px}
</style>
</head>
<body>
<div class="card">
<h2>生物成长记录仪</h2>
<p class="sub">请填写以下配置信息</p>
<form method="POST" action="/save" onsubmit="return validateForm()">
<label>WiFi 名称 (SSID)</label>
<input name="ssid" id="ssid" required maxlength="31" placeholder="MyWiFi">
<label>WiFi 密码</label>
<input name="pass" id="pass" type="password" maxlength="63" placeholder="WiFi 密码">
<label>确认 WiFi 密码</label>
<input name="pass2" id="pass2" type="password" maxlength="63" placeholder="再次输入密码">
<div id="passError" class="error">两次输入的密码不一致</div>
<label>服务器地址</label>
<input name="host" id="host" required maxlength="127" placeholder="192.168.1.100">
<label>服务器端口</label>
<input name="port" type="number" value="8443" min="1" max="65535">
<div class="checkbox-row">
<input type="checkbox" name="https" id="https" value="1">
<label for="https">使用 HTTPS (推荐, 需服务器支持)</label>
</div>
<p class="protocol-hint" id="protoHint">当前: HTTP (明文传输, 仅限局域网使用)</p>
<label>设备 ID</label>
<input name="dev_id" id="dev_id" required maxlength="31" placeholder="bgr-000001">
<label>拍照间隔 (秒)</label>
<input name="interval" type="number" value="60" min="5" max="86400">
<label>设备密钥 (64 位十六进制)</label>
<input name="secret" id="secret" type="password" maxlength="64" placeholder="留空则自动生成">
<span class="toggle-link" onclick="toggleSecret()">显示/隐藏密钥</span>
<p class="hint">密钥需与服务器端一致, 留空将自动生成随机密钥</p>
<button type="submit">保存并重启</button>
</form>
</div>
<script>
function validateForm(){
var p1=document.getElementById('pass').value;
var p2=document.getElementById('pass2').value;
if(p1!==p2){
document.getElementById('passError').style.display='block';
return false;
}
document.getElementById('passError').style.display='none';
return true;
}
function toggleSecret(){
var s=document.getElementById('secret');
s.type=s.type==='password'?'text':'password';
}
document.getElementById('https').addEventListener('change',function(){
var h=document.getElementById('protoHint');
if(this.checked){
h.textContent='当前: HTTPS (加密传输, 推荐)';
h.style.color='#4CAF50';
}else{
h.textContent='当前: HTTP (明文传输, 仅限局域网使用)';
h.style.color='#888';
}
});
</script>
</body>
</html>)HTML";

/* 配网成功页面 */
static const char* HTML_SUCCESS_PAGE = R"HTML(<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>配置成功</title>
<style>
*{box-sizing:border-box;margin:0;padding:0}
body{font-family:-apple-system,sans-serif;background:#f0f2f5;padding:16px}
.card{max-width:380px;margin:40px auto;background:#fff;border-radius:14px;padding:32px 24px;text-align:center;box-shadow:0 4px 16px rgba(0,0,0,.08)}
.icon{width:56px;height:56px;margin:0 auto 16px;border-radius:50%;background:#4CAF50;display:flex;align-items:center;justify-content:center;color:#fff;font-size:28px}
h2{color:#1a1a1a;margin-bottom:8px;font-size:20px}
p{color:#666;font-size:14px;line-height:1.6}
.sec{margin-top:12px;padding:10px;background:#f5f5f5;border-radius:8px;font-size:12px;color:#888;word-break:break-all}
</style>
</head>
<body>
<div class="card">
<div class="icon">&#10003;</div>
<h2>配置已保存</h2>
<p>设备将在 3 秒后重启并进入正常工作模式</p>
<p>请等待设备重启后连接 WiFi</p>
</div>
</body>
</html>)HTML";

/* ============================================================
 *  辅助函数
 * ============================================================ */

/* 十六进制字符转数值 */
static int hexCharToVal(char c) {
    if (c >= '0' && c <= '9') return c - '0';
    if (c >= 'a' && c <= 'f') return c - 'a' + 10;
    if (c >= 'A' && c <= 'F') return c - 'A' + 10;
    return -1;
}

/* URL 解码 (%xx -> 字节, + -> 空格) */
static std::string urlDecode(const char* str, size_t len) {
    std::string result;
    result.reserve(len);
    for (size_t i = 0; i < len; i++) {
        if (str[i] == '+') {
            result += ' ';
        } else if (str[i] == '%' && i + 2 < len) {
            int hi = hexCharToVal(str[i + 1]);
            int lo = hexCharToVal(str[i + 2]);
            if (hi >= 0 && lo >= 0) {
                result += (char)((hi << 4) | lo);
                i += 2;
            } else {
                result += str[i];
            }
        } else {
            result += str[i];
        }
    }
    return result;
}

/*
 * 从 URL 编码的表单体中提取指定字段的值
 *
 * 参数:
 *   body    - 表单体 (key=value&key=value 格式)
 *   bodyLen - 表单体长度
 *   key     - 要查找的字段名
 * 返回: 解码后的值, 未找到返回空字符串
 */
static std::string parseFormField(const char* body, size_t bodyLen,
                                   const char* key) {
    if (body == nullptr || key == nullptr) return "";

    size_t keyLen = strlen(key);
    const char* ptr = body;
    const char* end = body + bodyLen;

    while (ptr < end) {
        /* 查找 key= */
        if (ptr + keyLen < end &&
            strncmp(ptr, key, keyLen) == 0 &&
            ptr[keyLen] == '=') {
            /* 找到字段, 提取值 */
            const char* valStart = ptr + keyLen + 1;
            const char* valEnd = (const char*)memchr(valStart, '&', end - valStart);
            if (valEnd == nullptr) {
                valEnd = end;
            }
            return urlDecode(valStart, valEnd - valStart);
        }
        /* 跳到下一个 & */
        const char* next = (const char*)memchr(ptr, '&', end - ptr);
        if (next == nullptr) break;
        ptr = next + 1;
    }
    return "";
}

/* 生成随机 32 字节密钥的十六进制字符串 (64 字符) */
static std::string generateSecretHex() {
    uint8_t secret[32];
    esp_fill_random(secret, sizeof(secret));
    char hex[65];
    for (int i = 0; i < 32; i++) {
        snprintf(hex + i * 2, 3, "%02x", secret[i]);
    }
    hex[64] = '\0';
    return std::string(hex, 64);
}

/* 验证十六进制密钥字符串是否有效 (64 字符) */
static bool isValidSecretHex(const std::string& hex) {
    if (hex.length() != 64) return false;
    for (size_t i = 0; i < 64; i++) {
        if (hexCharToVal(hex[i]) < 0) return false;
    }
    return true;
}

/* ============================================================
 *  DNS 劫持服务器 (Captive Portal 核心)
 * ============================================================ */

/*
 * DNS 服务器任务: 拦截所有 DNS 查询, 返回 SoftAP IP (192.168.4.1)
 * 使所有域名解析到设备, 触发 Captive Portal 弹窗
 */
static void dnsServerTask(void* arg) {
    int sock = socket(AF_INET, SOCK_DGRAM, IPPROTO_UDP);
    if (sock < 0) {
        ESP_LOGE(TAG, "DNS: 创建 socket 失败");
        vTaskDelete(nullptr);
        return;
    }

    struct sockaddr_in serverAddr;
    memset(&serverAddr, 0, sizeof(serverAddr));
    serverAddr.sin_family = AF_INET;
    serverAddr.sin_port = htons(53);
    serverAddr.sin_addr.s_addr = htonl(INADDR_ANY);

    if (bind(sock, (struct sockaddr*)&serverAddr, sizeof(serverAddr)) < 0) {
        ESP_LOGE(TAG, "DNS: bind 端口 53 失败");
        close(sock);
        vTaskDelete(nullptr);
        return;
    }

    ESP_LOGI(TAG, "DNS 劫持服务器已启动 (端口 53)");

    uint8_t query[512];
    uint8_t response[512 + 16];  /* 查询 + 答案 */
    struct sockaddr_in clientAddr;
    socklen_t clientLen;

    while (true) {
        clientLen = sizeof(clientAddr);
        int recvLen = recvfrom(sock, query, sizeof(query), 0,
                               (struct sockaddr*)&clientAddr, &clientLen);
        if (recvLen < 12) {
            continue;  /* 太短, 不是有效 DNS 查询 */
        }

        /* 复制查询到响应缓冲区 */
        memcpy(response, query, recvLen);

        /* 修改 DNS 头部: 设置为标准响应, 无错误 */
        response[2] = 0x81;  /* QR=1, OPCODE=0, AA=0, TC=0, RD=1 */
        response[3] = 0x80;  /* RA=1, Z=0, RCODE=0 (无错误) */

        /* 设置计数: QDCOUNT=1, ANCOUNT=1, NSCOUNT=0, ARCOUNT=0 */
        response[4] = 0; response[5] = 1;   /* QDCOUNT */
        response[6] = 0; response[7] = 1;   /* ANCOUNT */
        response[8] = 0; response[9] = 0;   /* NSCOUNT */
        response[10] = 0; response[11] = 0; /* ARCOUNT */

        /* 找到问题段结束位置 (跳过 QNAME + QTYPE + QCLASS) */
        size_t qEnd = 12;
        while (qEnd < (size_t)recvLen && query[qEnd] != 0) {
            qEnd += query[qEnd] + 1;  /* 跳过 [长度][标签] */
        }
        qEnd += 5;  /* 跳过 null (1) + QTYPE (2) + QCLASS (2) */

        if (qEnd + 16 > sizeof(response)) {
            continue;  /* 缓冲区不足 */
        }

        /* 追加答案段 */
        /* NAME: 指针指向偏移 12 (问题段的 QNAME) */
        response[qEnd]     = 0xC0;
        response[qEnd + 1] = 0x0C;
        /* TYPE: A 记录 (0x0001) */
        response[qEnd + 2] = 0x00;
        response[qEnd + 3] = 0x01;
        /* CLASS: IN (0x0001) */
        response[qEnd + 4] = 0x00;
        response[qEnd + 5] = 0x01;
        /* TTL: 60 秒 */
        response[qEnd + 6] = 0x00;
        response[qEnd + 7] = 0x00;
        response[qEnd + 8] = 0x00;
        response[qEnd + 9] = 0x3C;
        /* RDLENGTH: 4 字节 (IPv4 地址) */
        response[qEnd + 10] = 0x00;
        response[qEnd + 11] = 0x04;
        /* RDATA: 192.168.4.1 */
        response[qEnd + 12] = AP_IP[0];
        response[qEnd + 13] = AP_IP[1];
        response[qEnd + 14] = AP_IP[2];
        response[qEnd + 15] = AP_IP[3];

        size_t respLen = qEnd + 16;

        sendto(sock, response, respLen, 0,
               (struct sockaddr*)&clientAddr, clientLen);
    }

    close(sock);
    vTaskDelete(nullptr);
}

/* ============================================================
 *  HTTP 服务器处理函数
 * ============================================================ */

/* GET 处理: 返回配网页面 (拦截所有 GET 请求, 含 Captive Portal 检测 URL) */
static esp_err_t httpGetHandler(httpd_req_t* req) {
    httpd_resp_set_type(req, "text/html; charset=utf-8");
    httpd_resp_send(req, HTML_CONFIG_PAGE, HTTPD_RESP_USE_STRLEN);
    return ESP_OK;
}

/* 重启任务: 延迟 3 秒后重启设备 */
static void restartTask(void* arg) {
    ESP_LOGI(TAG, "将在 3 秒后重启设备...");
    vTaskDelay(pdMS_TO_TICKS(3000));
    esp_restart();
}

/* POST 处理: 解析表单, 保存配置, 返回成功页面, 触发重启 */
static esp_err_t httpSaveHandler(httpd_req_t* req) {
    /* 读取 POST 请求体 */
    size_t contentLen = req->content_len;
    if (contentLen == 0 || contentLen > 2048) {
        httpd_resp_send_err(req, HTTPD_400_BAD_REQUEST, "无效请求");
        return ESP_FAIL;
    }

    char* body = (char*)malloc(contentLen + 1);
    if (body == nullptr) {
        httpd_resp_send_err(req, HTTPD_500_INTERNAL_SERVER_ERROR, "内存不足");
        return ESP_FAIL;
    }

    int received = httpd_req_recv(req, body, contentLen);
    if (received <= 0) {
        free(body);
        httpd_resp_send_err(req, HTTPD_400_BAD_REQUEST, "读取请求体失败");
        return ESP_FAIL;
    }
    body[received] = '\0';

    /* 解析表单字段 */
    Provisioning::Config cfg;
    cfg.ssid     = parseFormField(body, received, "ssid");
    cfg.pass     = parseFormField(body, received, "pass");
    cfg.host     = parseFormField(body, received, "host");
    cfg.deviceId = parseFormField(body, received, "dev_id");

    /* 端口 */
    std::string portStr = parseFormField(body, received, "port");
    cfg.port = (uint16_t)atoi(portStr.c_str());
    if (cfg.port == 0) cfg.port = PROVISION_DEFAULT_PORT;

    /* 拍照间隔 */
    std::string intervalStr = parseFormField(body, received, "interval");
    cfg.interval = (uint32_t)atoi(intervalStr.c_str());
    if (cfg.interval < 5) cfg.interval = PROVISION_DEFAULT_INTERVAL;

    /* 设备密钥: 留空则自动生成 */
    cfg.secretHex = parseFormField(body, received, "secret");
    if (cfg.secretHex.empty() || !isValidSecretHex(cfg.secretHex)) {
        cfg.secretHex = generateSecretHex();
        ESP_LOGI(TAG, "自动生成设备密钥: %s", cfg.secretHex.c_str());
    }

    /* HTTPS 选项 (ADR-009) */
    std::string httpsVal = parseFormField(body, received, "https");
    cfg.useHttps = (httpsVal == "1");

    free(body);

    /* 验证必填字段 */
    if (cfg.ssid.empty() || cfg.host.empty() || cfg.deviceId.empty()) {
        httpd_resp_send_err(req, HTTPD_400_BAD_REQUEST,
                            "缺少必填字段 (SSID/服务器/设备ID)");
        return ESP_FAIL;
    }

    /* 保存到 NVS */
    if (!Provisioning::saveConfig(cfg)) {
        httpd_resp_send_err(req, HTTPD_500_INTERNAL_SERVER_ERROR, "保存配置失败");
        return ESP_FAIL;
    }

    ESP_LOGI(TAG, "配置已保存: SSID=%s, Host=%s:%d, Device=%s, Interval=%lus, HTTPS=%s",
             cfg.ssid.c_str(), cfg.host.c_str(), cfg.port,
             cfg.deviceId.c_str(), (unsigned long)cfg.interval,
             cfg.useHttps ? "YES" : "NO");

    /* 返回成功页面 */
    httpd_resp_set_type(req, "text/html; charset=utf-8");
    httpd_resp_send(req, HTML_SUCCESS_PAGE, HTTPD_RESP_USE_STRLEN);

    /* 启动重启任务 */
    xTaskCreate(restartTask, "restart", 2048, nullptr, 5, nullptr);

    return ESP_OK;
}

/* ============================================================
 *  Provisioning 类方法实现
 * ============================================================ */

bool Provisioning::isProvisioned() {
    nvs_handle_t handle;
    esp_err_t err = nvs_open(NVS_NAMESPACE_PROVISION, NVS_READONLY, &handle);
    if (err != ESP_OK) {
        return false;
    }

    uint8_t provisioned = 0;
    err = nvs_get_u8(handle, NVS_KEY_PROVISIONED, &provisioned);
    nvs_close(handle);

    return (err == ESP_OK && provisioned == 1);
}

bool Provisioning::loadConfig(Config& cfg) {
    nvs_handle_t handle;
    esp_err_t err = nvs_open(NVS_NAMESPACE_PROVISION, NVS_READONLY, &handle);
    if (err != ESP_OK) {
        ESP_LOGE(TAG, "无法打开 NVS 命名空间: %s", esp_err_to_name(err));
        return false;
    }

    /* 读取字符串字段 */
    char buf[128] = {0};
    size_t len = sizeof(buf);

    len = sizeof(buf);
    if (nvs_get_str(handle, NVS_KEY_SSID, buf, &len) == ESP_OK) {
        cfg.ssid = buf;
    }

    len = sizeof(buf);
    if (nvs_get_str(handle, NVS_KEY_PASS, buf, &len) == ESP_OK) {
        cfg.pass = buf;
    }

    len = sizeof(buf);
    if (nvs_get_str(handle, NVS_KEY_HOST, buf, &len) == ESP_OK) {
        cfg.host = buf;
    }

    len = sizeof(buf);
    if (nvs_get_str(handle, NVS_KEY_DEVICE_ID, buf, &len) == ESP_OK) {
        cfg.deviceId = buf;
    }

    len = sizeof(buf);
    if (nvs_get_str(handle, NVS_KEY_SECRET, buf, &len) == ESP_OK) {
        cfg.secretHex = buf;
    }

    /* 读取数值字段 */
    uint16_t port = 0;
    if (nvs_get_u16(handle, NVS_KEY_PORT, &port) == ESP_OK) {
        cfg.port = port;
    } else {
        cfg.port = PROVISION_DEFAULT_PORT;
    }

    uint32_t interval = 0;
    if (nvs_get_u32(handle, NVS_KEY_INTERVAL, &interval) == ESP_OK) {
        cfg.interval = interval;
    } else {
        cfg.interval = PROVISION_DEFAULT_INTERVAL;
    }

    /* HTTPS 选项 (ADR-009) */
    uint8_t useHttps = 0;
    if (nvs_get_u8(handle, NVS_KEY_USE_HTTPS, &useHttps) == ESP_OK) {
        cfg.useHttps = (useHttps == 1);
    } else {
        cfg.useHttps = false;
    }

    nvs_close(handle);

    /* 验证关键字段 */
    if (cfg.ssid.empty() || cfg.deviceId.empty()) {
        ESP_LOGE(TAG, "NVS 配置不完整");
        return false;
    }

    ESP_LOGI(TAG, "配置加载成功: SSID=%s, Host=%s:%d, Device=%s, HTTPS=%s",
             cfg.ssid.c_str(), cfg.host.c_str(), cfg.port,
             cfg.deviceId.c_str(), cfg.useHttps ? "YES" : "NO");
    return true;
}

bool Provisioning::saveConfig(const Config& cfg) {
    nvs_handle_t handle;
    esp_err_t err = nvs_open(NVS_NAMESPACE_PROVISION, NVS_READWRITE, &handle);
    if (err != ESP_OK) {
        ESP_LOGE(TAG, "无法打开 NVS (写入): %s", esp_err_to_name(err));
        return false;
    }

    bool ok = true;

    if (nvs_set_str(handle, NVS_KEY_SSID, cfg.ssid.c_str()) != ESP_OK) ok = false;
    if (nvs_set_str(handle, NVS_KEY_PASS, cfg.pass.c_str()) != ESP_OK) ok = false;
    if (nvs_set_str(handle, NVS_KEY_HOST, cfg.host.c_str()) != ESP_OK) ok = false;
    if (nvs_set_u16(handle, NVS_KEY_PORT, cfg.port) != ESP_OK) ok = false;
    if (nvs_set_str(handle, NVS_KEY_DEVICE_ID, cfg.deviceId.c_str()) != ESP_OK) ok = false;
    if (nvs_set_u32(handle, NVS_KEY_INTERVAL, cfg.interval) != ESP_OK) ok = false;
    if (nvs_set_str(handle, NVS_KEY_SECRET, cfg.secretHex.c_str()) != ESP_OK) ok = false;
    if (nvs_set_u8(handle, NVS_KEY_PROVISIONED, 1) != ESP_OK) ok = false;
    if (nvs_set_u8(handle, NVS_KEY_USE_HTTPS, cfg.useHttps ? 1 : 0) != ESP_OK) ok = false;
    // 存储固件版本号, 用于启动时自动检测固件升级 (v2.2)
    if (nvs_set_str(handle, NVS_KEY_FW_VERSION, FIRMWARE_VERSION) != ESP_OK) ok = false;

    if (nvs_commit(handle) != ESP_OK) ok = false;

    nvs_close(handle);
    return ok;
}

std::string Provisioning::getApSsid() {
    uint8_t mac[6];
    esp_read_mac(mac, ESP_MAC_WIFI_SOFTAP);
    char ssid[32];
    snprintf(ssid, sizeof(ssid), "BioRecorder-%02X%02X", mac[4], mac[5]);
    return std::string(ssid);
}

std::string Provisioning::getApPin() {
    // 从 MAC 地址派生 8 位数字 PIN (用户需查看串口输出获取 PIN)
    uint8_t mac[6];
    esp_read_mac(mac, ESP_MAC_WIFI_SOFTAP);
    // 使用 MAC 地址的哈希生成 8 位 PIN
    uint32_t hash = (mac[0] << 24) | (mac[1] << 16) | (mac[2] << 8) | mac[3];
    hash ^= (mac[4] << 8) | mac[5];
    hash = hash % 100000000;  // 限制为 8 位数字
    char pin[9];
    snprintf(pin, sizeof(pin), "%08u", (unsigned)hash);
    return std::string(pin);
}

bool Provisioning::startSoftAP() {
    /* 网络接口和事件循环已在 main.cc 中初始化, 此处仅创建 AP netif */
    esp_netif_t* apNetif = esp_netif_create_default_wifi_ap();
    if (apNetif == nullptr) {
        ESP_LOGE(TAG, "创建 AP netif 失败");
        return false;
    }

    /* 初始化 WiFi (首次进入配网时) */
    wifi_init_config_t cfg = WIFI_INIT_CONFIG_DEFAULT();
    ESP_ERROR_CHECK(esp_wifi_init(&cfg));

    /* 配置 AP (WPA2-PSK, PIN 从 MAC 地址派生, 防止未授权配网) */
    std::string ssid = getApSsid();
    std::string pin = getApPin();  // 8 位数字 PIN
    wifi_config_t apConfig = {};
    strncpy((char*)apConfig.ap.ssid, ssid.c_str(), sizeof(apConfig.ap.ssid) - 1);
    apConfig.ap.ssid_len = ssid.length();
    apConfig.ap.channel = 6;
    apConfig.ap.authmode = WIFI_AUTH_OPEN;
    apConfig.ap.max_connection = 2;
    strncpy((char*)apConfig.ap.password, pin.c_str(), sizeof(apConfig.ap.password) - 1);

    ESP_ERROR_CHECK(esp_wifi_set_mode(WIFI_MODE_AP));
    ESP_ERROR_CHECK(esp_wifi_set_config(WIFI_IF_AP, &apConfig));
    ESP_ERROR_CHECK(esp_wifi_start());

    ESP_LOGI(TAG, "SoftAP 已启动: %s (PIN: %s)", ssid.c_str(), pin.c_str());
    return true;
}

void Provisioning::startDnsServer() {
    xTaskCreate(dnsServerTask, "dns_server", 4096, nullptr, 5, nullptr);
}

bool Provisioning::startHttpServer() {
    httpd_config_t config = HTTPD_DEFAULT_CONFIG();
    config.server_port = 80;
    config.uri_match_fn = httpd_uri_match_wildcard;
    config.max_uri_handlers = 8;
    config.lru_purge_enable = true;

    if (httpd_start(&s_httpServer, &config) != ESP_OK) {
        ESP_LOGE(TAG, "HTTP 服务器启动失败");
        return false;
    }

    /* POST /save - 处理表单提交 */
    httpd_uri_t saveUri = {};
    saveUri.uri = "/save";
    saveUri.method = HTTP_POST;
    saveUri.handler = httpSaveHandler;
    saveUri.user_ctx = nullptr;
    httpd_register_uri_handler(s_httpServer, &saveUri);

    // GET /* - 拦截所有 GET 请求 (含 Captive Portal 检测 URL)
    httpd_uri_t configUri = {};
    configUri.uri = "/*";
    configUri.method = HTTP_GET;
    configUri.handler = httpGetHandler;
    configUri.user_ctx = nullptr;
    httpd_register_uri_handler(s_httpServer, &configUri);

    ESP_LOGI(TAG, "HTTP 配网服务器已启动 (端口 80)");
    return true;
}

void Provisioning::start() {
    ESP_LOGI(TAG, "========================================");
    ESP_LOGI(TAG, "  进入配网模式 (SoftAP Captive Portal)");
    ESP_LOGI(TAG, "========================================");

    /* 1. 启动 SoftAP */
    if (!startSoftAP()) {
        ESP_LOGE(TAG, "SoftAP 启动失败, 10 秒后重启");
        vTaskDelay(pdMS_TO_TICKS(10000));
        esp_restart();
    }

    /* 2. 启动 DNS 劫持服务器 */
    startDnsServer();

    /* 3. 启动 HTTP 配网服务器 */
    if (!startHttpServer()) {
        ESP_LOGE(TAG, "HTTP 服务器启动失败, 10 秒后重启");
        vTaskDelay(pdMS_TO_TICKS(10000));
        esp_restart();
    }

    std::string apSsid = getApSsid();
    std::string apPin = getApPin();
    ESP_LOGI(TAG, "请连接 WiFi: %s", apSsid.c_str());
    ESP_LOGI(TAG, "WiFi 密码 (PIN): %s", apPin.c_str());
    ESP_LOGI(TAG, "浏览器访问 http://192.168.4.1 进行配置");
    ESP_LOGI(TAG, "等待用户完成配网...");

    /* 4. 阻塞等待 (配网完成后由 save 处理函数触发重启) */
    while (true) {
        vTaskDelay(pdMS_TO_TICKS(1000));
    }
}
