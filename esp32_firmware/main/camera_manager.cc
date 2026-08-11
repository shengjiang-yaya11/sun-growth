/*
 * ============================================================
 *  【ESP32 端固件】烧录目标: ESP32-S3 CAM V1.1 开发板
 *  编译工具链: ESP-IDF v5.x + Xtensa gcc (不依赖电脑端软件)
 *  烧录方式: USB 连接开发板, 运行 idf.py flash
 * ============================================================
 *
 * camera_manager.cc - OV3660 摄像头管理器实现
 *
 * 引脚: AI-Thinker / GOOUUU ESP32-S3-CAM 标准 (实测可用)
 * 参考: CSDN ESP32-S3-CAM 接 OV3660 实测 + DFRobot DFR1154 调优
 */

#include "camera_manager.h"
#include "config.h"
#include "esp_log.h"
#include "esp_psram.h"
#include <cstring>

static const char* TAG = "CAM";
bool CameraManager::initialized_ = false;

// AI-Thinker 标准 OV3660 引脚
#define CAM_PIN_PWDN    -1
#define CAM_PIN_RESET   -1
#define CAM_PIN_XCLK    15
#define CAM_PIN_SIOD     4
#define CAM_PIN_SIOC     5
#define CAM_PIN_Y9      16
#define CAM_PIN_Y8      17
#define CAM_PIN_Y7      18
#define CAM_PIN_Y6      12
#define CAM_PIN_Y5      10
#define CAM_PIN_Y4       8
#define CAM_PIN_Y3       9
#define CAM_PIN_Y2      11
#define CAM_PIN_VSYNC    6
#define CAM_PIN_HREF     7
#define CAM_PIN_PCLK    13

bool CameraManager::init() {
    if (initialized_) return true;

    camera_config_t cfg = {};
    cfg.ledc_channel = LEDC_CHANNEL_0;
    cfg.ledc_timer   = LEDC_TIMER_0;
    cfg.pin_pwdn     = CAM_PIN_PWDN;
    cfg.pin_reset    = CAM_PIN_RESET;
    cfg.pin_xclk     = CAM_PIN_XCLK;
    cfg.pin_sccb_sda = CAM_PIN_SIOD;
    cfg.pin_sccb_scl = CAM_PIN_SIOC;
    cfg.pin_d7       = CAM_PIN_Y9;
    cfg.pin_d6       = CAM_PIN_Y8;
    cfg.pin_d5       = CAM_PIN_Y7;
    cfg.pin_d4       = CAM_PIN_Y6;
    cfg.pin_d3       = CAM_PIN_Y5;
    cfg.pin_d2       = CAM_PIN_Y4;
    cfg.pin_d1       = CAM_PIN_Y3;
    cfg.pin_d0       = CAM_PIN_Y2;
    cfg.pin_vsync    = CAM_PIN_VSYNC;
    cfg.pin_href     = CAM_PIN_HREF;
    cfg.pin_pclk     = CAM_PIN_PCLK;

    cfg.xclk_freq_hz = CAM_XCLK_FREQ;
    cfg.pixel_format = PIXFORMAT_JPEG;
    cfg.frame_size   = CAM_FRAME_SIZE;
    cfg.jpeg_quality = CAM_JPEG_QUALITY;

    // PSRAM 双缓冲 (8MB PSRAM 在 N16R8 板上)
    if (esp_psram_is_initialized()) {
        cfg.fb_location = CAMERA_FB_IN_PSRAM;
        cfg.fb_count    = 2;
        cfg.grab_mode   = CAMERA_GRAB_LATEST;
        ESP_LOGI(TAG, "PSRAM 检测到, 使用双缓冲");
    } else {
        cfg.fb_location = CAMERA_FB_IN_DRAM;
        cfg.fb_count    = 1;
        cfg.grab_mode   = CAMERA_GRAB_WHEN_EMPTY;
        cfg.frame_size  = FRAMESIZE_SVGA;
        ESP_LOGW(TAG, "无 PSRAM, 降级到 SVGA");
    }

    esp_err_t err = esp_camera_init(&cfg);
    if (err != ESP_OK) {
        ESP_LOGE(TAG, "初始化失败: 0x%x", err);
        return false;
    }

    // OV3660 传感器调优 (参考 DFRobot DFR1154)
    sensor_t* s = esp_camera_sensor_get();
    if (s) {
        ESP_LOGI(TAG, "传感器 PID: 0x%x", s->id.PID);
        if (s->id.PID == OV3660_PID) {
            s->set_vflip(s, 1);
            s->set_hmirror(s, 0);
            s->set_brightness(s, 1);
            s->set_saturation(s, -1);
            s->set_gainceiling(s, GAINCEILING_8X);
            ESP_LOGI(TAG, "OV3660 调优完成");
        } else if (s->id.PID == OV2640_PID) {
            s->set_vflip(s, 0);
            s->set_hmirror(s, 0);
            ESP_LOGI(TAG, "OV2640 配置完成");
        }
    }

    initialized_ = true;
    ESP_LOGI(TAG, "摄像头初始化成功");
    return true;
}

camera_fb_t* CameraManager::capture() {
    camera_fb_t* fb = esp_camera_fb_get();
    if (!fb) {
        ESP_LOGE(TAG, "拍照失败");
        return nullptr;
    }
    ESP_LOGI(TAG, "拍照: %dx%d, %u B", fb->width, fb->height, (unsigned)fb->len);
    return fb;
}

void CameraManager::returnFrame(camera_fb_t* fb) {
    if (fb) esp_camera_fb_return(fb);
}

void CameraManager::deinit() {
    if (initialized_) {
        esp_camera_deinit();
        initialized_ = false;
        ESP_LOGI(TAG, "摄像头已反初始化");
    }
}
