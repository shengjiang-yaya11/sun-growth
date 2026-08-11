/*
 * ============================================================
 *  【ESP32 端固件】烧录目标: ESP32-S3 CAM V1.1 开发板
 *  编译工具链: ESP-IDF v5.x + Xtensa gcc (不依赖电脑端软件)
 *  烧录方式: USB 连接开发板, 运行 idf.py flash
 * ============================================================
 *
 * provisioning.h - SoftAP Captive Portal 配网模块
 *
 * 首次启动 (NVS 无配网标志) 时:
 *   1. 开启 SoftAP (SSID: BioRecorder-XXXX, XXXX=MAC后4位)
 *   2. 启动 DNS 劫持服务器 (拦截所有域名解析返回设备 IP)
 *   3. 启动 HTTP 服务器 (端口 80) 提供配网页面
 *   4. 用户通过 Captive Portal 填写 WiFi/服务器/设备配置
 *   5. 配置保存到 NVS, 设备重启进入正常模式
 *
 * NVS 命名空间: "provision"
 * NVS 键: ssid, pass, host, port, dev_id, interval, secret, provisioned
 *
 * ADR-003: ESP32 设备配网改用 SoftAP Captive Portal
 */

#pragma once

#include <stdint.h>
#include <stddef.h>
#include <string>

class Provisioning {
public:
    /* 设备配置结构体 */
    struct Config {
        std::string ssid;       /* WiFi SSID */
        std::string pass;       /* WiFi 密码 */
        std::string host;       /* 服务器地址 */
        uint16_t    port;       /* 服务器端口 */
        std::string deviceId;   /* 设备 ID */
        uint32_t    interval;   /* 拍照间隔 (秒) */
        std::string secretHex;  /* 设备密钥 (64 位十六进制字符串) */
        bool        useHttps;   /* 是否使用 HTTPS 上传 (ADR-009) */
    };

    /*
     * 检查设备是否已配网
     * 返回: true = NVS 中 provisioned 标志为 1
     */
    static bool isProvisioned();

    /*
     * 从 NVS 加载完整配置
     * 参数:
     *   cfg - 输出配置结构体
     * 返回: true = 加载成功且配置有效
     */
    static bool loadConfig(Config& cfg);

    /*
     * 启动配网模式 (SoftAP + Captive Portal)
     *
     * 此函数阻塞调用线程, 直到用户完成配网后设备自动重启
     * 配网流程:
     *   - 初始化网络 (netif + event loop)
     *   - 启动 SoftAP
     *   - 启动 DNS 劫持服务器 (端口 53)
     *   - 启动 HTTP 服务器 (端口 80)
     *   - 等待用户提交配置
     *   - 保存到 NVS 并重启
     */
    static void start();

    /*
     * 保存配置到 NVS (配网页面表单提交时调用)
     * 参数:
     *   cfg - 要保存的配置
     * 返回: true = 保存成功
     */
    static bool saveConfig(const Config& cfg);

private:
    /* 获取 SoftAP SSID (BioRecorder-XXXX) */
    static std::string getApSsid();

    /* 获取 SoftAP PIN (8 位数字, 从 MAC 地址派生) */
    static std::string getApPin();

    /* 启动 SoftAP 热点 */
    static bool startSoftAP();

    /* 启动 DNS 劫持服务器 (Captive Portal 核心组件) */
    static void startDnsServer();

    /* 启动 HTTP 配网服务器 */
    static bool startHttpServer();
};
