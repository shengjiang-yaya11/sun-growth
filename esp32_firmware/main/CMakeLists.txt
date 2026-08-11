# ============================================================
#  【ESP32 端固件】烧录目标: ESP32-S3 CAM V1.1 开发板
#  编译工具链: ESP-IDF v5.x + Xtensa gcc (不依赖电脑端软件)
#  烧录方式: USB 连接开发板, 运行 idf.py flash
# ============================================================
#
# main/CMakeLists.txt - 主应用组件
#
# 纯 ESP-IDF 项目, 无 Rust 依赖 (ADR-001)
# 依赖: bio_security (mbedTLS C 安全模块), provisioning (SoftAP Captive Portal)

idf_component_register(
    SRCS "main.cc"
          "wifi_manager.cc"
          "camera_manager.cc"
          "uploader.cc"
          "retry_queue.cc"
          "security_bridge.cc"
          "provisioning.cc"
          "command_server.cc"
    INCLUDE_DIRS "."
    REQUIRES bio_security
             esp_wifi
             esp_http_client
             esp_http_server
             esp_camera
             nvs_flash
             esp_timer
             esp_hw_support
             esp_psram
             esp_event
             esp_netif
             led_strip
             driver
)
