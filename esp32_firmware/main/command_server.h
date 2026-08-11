/*
 * ============================================================
 *  command_server.h - ESP32 命令接收服务器 (v3.1)
 *
 *  功能: 在 ESP32 端运行 TCP 服务器, 接收电脑端下发的控制命令
 *
 *  双向通信架构:
 *    PC → ESP32:  JSON 命令 (TCP port 8081)
 *    ESP32 → PC:  JSON 响应 + 加密照片 (HTTP POST /api/v1/upload)
 *
 *  支持命令:
 *    capture     - 立即拍照并上传
 *    status      - 查询设备状态
 *    set_config  - 修改拍照间隔等参数
 *    restart     - 重启设备
 *
 *  安全: HMAC-SHA256 签名验证 (与上传使用相同密钥)
 * ============================================================
 */

#pragma once

#include <string>
#include <functional>

// 命令回调类型: 参数 JSON 字符串, 返回响应 JSON 字符串
using CommandHandler = std::function<std::string(const std::string&)>;

class CommandServer {
public:
    // 启动 TCP 命令服务器 (非阻塞, 创建独立任务)
    // port: 监听端口 (默认 8081)
    static void start(uint16_t port = 8081);

    // 停止服务器
    static void stop();

    // 服务器是否在运行
    static bool isRunning();

    // 注册自定义命令处理器
    // cmd: 命令名 (如 "capture", "status")
    // handler: 处理器函数 (参数=请求JSON, 返回=响应JSON)
    static void registerHandler(const std::string& cmd, CommandHandler handler);

    // 获取服务器监听端口
    static uint16_t getPort();

    // 构建标准 JSON 响应 (v3.1: public, 供 main.cc 的 capture handler 使用)
    static std::string buildResponse(const std::string& status,
                                     const std::string& data);

private:
    static void serverTask(void* arg);
    static std::string processCommand(const std::string& requestJson);
    static bool verifySignature(const std::string& body);
};
