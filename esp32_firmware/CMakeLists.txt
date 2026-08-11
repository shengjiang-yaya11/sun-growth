# ============================================================
#  【ESP32 端固件】烧录目标: ESP32-S3 CAM V1.1 开发板
#  编译工具链: ESP-IDF v5.x + Xtensa gcc (不依赖电脑端软件)
#  烧录方式: USB 连接开发板, 运行 idf.py flash
# ============================================================
#
# CMakeLists.txt - ESP32-S3 生物成长记录仪主项目
#
# 纯 ESP-IDF CMake 项目 (ADR-001: 移除 Rust 依赖)
# 安全模块使用 ESP-IDF 内置 mbedTLS, 无外部工具链依赖
#
# ESP-IDF v5.x

cmake_minimum_required(VERSION 3.16)

# 包含 ESP-IDF 构建系统
include($ENV{IDF_PATH}/tools/cmake/project.cmake)

project(bio_growth_recorder)
