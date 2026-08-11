/*
 * ============================================================
 *  【ESP32 端固件】烧录目标: ESP32-S3 CAM V1.1 开发板
 *  编译工具链: ESP-IDF v5.x + Xtensa gcc (不依赖电脑端软件)
 *  烧录方式: USB 连接开发板, 运行 idf.py flash
 * ============================================================
 *
 * wifi_manager.h - WiFi 连接管理
 */

#pragma once
#include <string>

class WifiManager {
public:
    /// 初始化 WiFi 子系统
    static bool init();

    /// 连接 WiFi (阻塞, 超时 20s)
    static bool connect();

    /// 确保已连接, 断开则重连
    static void ensureConnected();

    /// 是否已连接
    static bool isConnected();

    /// 设置连接状态 (事件处理器回调用)
    static void setConnected(bool v) { connected_ = v; }

    /// 获取 IP 地址
    static std::string getIP();

    /// 断开并关闭 WiFi (深度睡眠前)
    static void disconnect();

private:
    static bool initialized_;
    static bool connected_;
};
