/*
 * ============================================================
 *  【ESP32 端固件】烧录目标: ESP32-S3 CAM V1.1 开发板
 *  编译工具链: ESP-IDF v5.x + Xtensa gcc (不依赖电脑端软件)
 *  烧录方式: USB 连接开发板, 运行 idf.py flash
 * ============================================================
 *
 * config.h - 设备配置常量 (编译时固定值 + NVS 键名定义)
 *
 * 商用产品: WiFi/服务器/设备ID/密钥等敏感信息通过 SoftAP Captive Portal
 *           配网写入 NVS, 不硬编码在固件中 (ADR-003)
 *
 * 此文件仅保留: 摄像头配置、安全协议常量、NVS 键名、HTTP 超时等编译时固定值
 */

#pragma once
#include <stdint.h>

/* ============================================================
 *  NVS 配网存储 (ADR-003: SoftAP Captive Portal)
 * ============================================================ */

/* NVS 命名空间 */
#define NVS_NAMESPACE_PROVISION   "provision"

/* NVS 键名定义 */
#define NVS_KEY_SSID              "ssid"        /* WiFi SSID */
#define NVS_KEY_PASS              "pass"        /* WiFi 密码 */
#define NVS_KEY_HOST              "host"        /* 服务器地址 */
#define NVS_KEY_PORT              "port"        /* 服务器端口 (uint16_t) */
#define NVS_KEY_DEVICE_ID         "dev_id"      /* 设备 ID */
#define NVS_KEY_INTERVAL          "interval"    /* 拍照间隔秒数 (uint32_t) */
#define NVS_KEY_SECRET            "secret"      /* 设备密钥 (32字节十六进制, 64字符) */
#define NVS_KEY_PROVISIONED       "provisioned" /* 是否已配网 (uint8_t, 1=已配网) */
#define NVS_KEY_USE_HTTPS         "use_https"   /* 是否使用 HTTPS (uint8_t, 1=HTTPS) */

/* 配网默认值 (NVS 无值时使用) */
#define PROVISION_DEFAULT_PORT    8443           /* 默认服务器端口 */
#define PROVISION_DEFAULT_INTERVAL 60            /* 默认拍照间隔 (秒) */

/* ============================================================
 *  摄像头配置 (编译时固定)
 * ============================================================ */

#define CAM_FRAME_SIZE        FRAMESIZE_UXGA   /* 1600x1200 */
#define CAM_JPEG_QUALITY      12               /* 0-63, 越小越高 */
#define CAM_XCLK_FREQ         20000000         /* 20MHz */

/* ============================================================
 *  HTTP / 网络配置 (编译时固定)
 * ============================================================ */

/* 上传 API 路径 (非敏感, 固定值) */
#define DEFAULT_SERVER_PATH   "/api/v1/upload"

/* HTTP 超时 (毫秒) */
#define HTTP_TIMEOUT_MS       15000

/* WiFi 重连间隔 (毫秒) */
#define WIFI_RECONNECT_MS     10000

/* ============================================================
 *  运行模式 (编译时固定)
 * ============================================================ */

/* 深度睡眠模式 (0=常驻, 1=省电) */
#define DEFAULT_DEEP_SLEEP    0

/* 固件版本 (用于上报服务器 + NVS版本检测, 变更时自动清除旧配网) */
#define FIRMWARE_VERSION      "3.1.0"

/* NVS 版本键 (用于检测固件升级, 自动清除旧配网数据) */
#define NVS_KEY_FW_VERSION     "fw_ver"

/* ============================================================
 *  安全协议常量 (与 bio_security.h 保持一致)
 * ============================================================ */

#define SECURITY_NONCE_LEN    12
#define SECURITY_HASH_LEN     32
#define SECURITY_SIG_LEN      32
#define SECURITY_OVERHEAD     44   /* nonce(12) + tag(32) */

/* ============================================================
 *  工厂复位 (ADR-008: 长按 BOOT 键)
 * ============================================================ */

/* BOOT 键 GPIO (ESP32-S3 CAM 板载 BOOT 按键) */
#define BOOT_BUTTON_GPIO      0

/* 长按触发时间 (毫秒) */
#define FACTORY_RESET_HOLD_MS 10000

/* 轮询间隔 (毫秒) */
#define BOOT_BUTTON_POLL_MS   100

/* ============================================================
 *  堆内存监控 (ADR-010)
 * ============================================================ */

/* 监控间隔 (毫秒) */
#define HEAP_MONITOR_INTERVAL_MS  30000

/* 低内存告警阈值 (字节) */
#define HEAP_LOW_THRESHOLD       (50 * 1024)   /* 50 KB */

/* ============================================================
 *  重试队列配置 (ADR-007)
 * ============================================================ */

/* 重试间隔 (主循环中每次重试的间隔, 毫秒) */
#define RETRY_INTERVAL_MS     5000
