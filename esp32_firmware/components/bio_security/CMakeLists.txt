# ============================================================
#  【ESP32 端固件】烧录目标: ESP32-S3 CAM V1.1 开发板
#  编译工具链: ESP-IDF v5.x + Xtensa gcc (不依赖电脑端软件)
#  烧录方式: USB 连接开发板, 运行 idf.py flash
# ============================================================
#
# CMakeLists.txt - bio_security ESP-IDF 组件 (纯 C, mbedTLS 实现)
#
# 替代原 Rust no_std 安全模块 (ADR-001)
# 使用 ESP-IDF 内置 mbedTLS 提供 AES-256-CTR / HMAC-SHA256 / SHA-256

idf_component_register(
    SRCS "bio_security.c"
    INCLUDE_DIRS "."
    REQUIRES mbedtls
)
