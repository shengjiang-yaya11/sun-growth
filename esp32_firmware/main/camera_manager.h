/*
 * ============================================================
 *  【ESP32 端固件】烧录目标: ESP32-S3 CAM V1.1 开发板
 *  编译工具链: ESP-IDF v5.x + Xtensa gcc (不依赖电脑端软件)
 *  烧录方式: USB 连接开发板, 运行 idf.py flash
 * ============================================================
 *
 * camera_manager.h - OV3660 摄像头管理器
 */

#pragma once
#include "esp_camera.h"
#include <cstdint>

class CameraManager {
public:
    /// 初始化摄像头
    static bool init();

    /// 拍照, 返回帧缓冲 (调用方需调用 returnFrame 释放)
    static camera_fb_t* capture();

    /// 释放帧缓冲
    static void returnFrame(camera_fb_t* fb);

    /// 反初始化 (深度睡眠前调用)
    static void deinit();

private:
    static bool initialized_;
};
