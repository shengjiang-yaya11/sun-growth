#!/bin/bash
# ============================================================
#  ESP32 固件编译脚本 - 生成 bin 文件
#  使用 ESP-IDF Docker 镜像编译, 无需本地安装 ESP-IDF
#  作者: Andrew 亚生
# ============================================================
set -e

FIRMWARE_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$FIRMWARE_DIR")"
OUTPUT_DIR="$FIRMWARE_DIR/build_output"

echo "========================================================"
echo "  ESP32-S3 CAM 固件编译"
echo "  目标: 生成可烧录的 bin 文件"
echo "========================================================"
echo ""

# 方法 1: 使用 Docker 编译 (推荐, 无需安装 ESP-IDF)
if command -v docker &> /dev/null; then
    echo "[1/4] 检测到 Docker, 使用 Docker 编译..."
    echo ""

    # 创建输出目录
    mkdir -p "$OUTPUT_DIR"

    # 使用 ESP-IDF 官方 Docker 镜像编译
    docker run --rm \
        -v "$FIRMWARE_DIR:/project" \
        -v "$OUTPUT_DIR:/output" \
        -w /project \
        espressif/idf:v5.1.2 \
        bash -c '
            echo "[2/4] 设置 ESP-IDF 环境..."
            . /opt/esp/idf/export.sh

            echo "[3/4] 设置目标芯片 ESP32-S3..."
            idf.py set-target esp32s3

            echo "[4/4] 编译固件..."
            idf.py build

            echo ""
            echo "========================================================"
            echo "  编译完成! bin 文件位置:"
            echo "========================================================"

            # 列出生成的 bin 文件
            ls -lh build/*.bin

            # 复制 bin 文件到输出目录
            cp build/bio_growth_recorder.bin /output/bio-recorder-v3.0-esp32s3-firmware.bin
            cp build/bootloader/bootloader.bin /output/bio-recorder-v3.0-esp32s3-bootloader.bin
            cp build/partition_table/partition-table.bin /output/bio-recorder-v3.0-esp32s3-partitions.bin

            echo ""
            echo "bin 文件已复制到: $OUTPUT_DIR"
            echo ""
            echo "烧录命令:"
            echo "  esptool.py --chip esp32s3 --port /dev/ttyUSB0 --baud 921600 \\"
            echo "    write_flash 0x0 bio-recorder-v3.0-esp32s3-bootloader.bin \\"
            echo "    0x8000 bio-recorder-v3.0-esp32s3-partitions.bin \\"
            echo "    0x10000 bio-recorder-v3.0-esp32s3-firmware.bin"
        '

    echo ""
    echo "========================================================"
    echo "  编译完成!"
    echo "  bin 文件位于: $OUTPUT_DIR"
    echo "========================================================"
    ls -lh "$OUTPUT_DIR"/

# 方法 2: 使用本地 ESP-IDF 编译
elif [ -f "$HOME/esp/esp-idf/export.sh" ]; then
    echo "[1/4] 检测到本地 ESP-IDF..."
    . "$HOME/esp/esp-idf/export.sh"

    cd "$FIRMWARE_DIR"
    echo "[2/4] 设置目标芯片 ESP32-S3..."
    idf.py set-target esp32s3

    echo "[3/4] 编译固件..."
    idf.py build

    echo "[4/4] 生成 bin 文件..."
    mkdir -p "$OUTPUT_DIR"
    cp build/bio_growth_recorder.bin "$OUTPUT_DIR/bio-recorder-v3.0-esp32s3-firmware.bin"
    cp build/bootloader/bootloader.bin "$OUTPUT_DIR/bio-recorder-v3.0-esp32s3-bootloader.bin"
    cp build/partition_table/partition-table.bin "$OUTPUT_DIR/bio-recorder-v3.0-esp32s3-partitions.bin"

    echo ""
    echo "========================================================"
    echo "  编译完成!"
    echo "  bin 文件位于: $OUTPUT_DIR"
    echo "========================================================"
    ls -lh "$OUTPUT_DIR"/

else
    echo "错误: 未检测到 Docker 或 ESP-IDF"
    echo ""
    echo "请选择以下方式之一:"
    echo ""
    echo "方式 1: 安装 Docker 后运行此脚本"
    echo "  Docker: https://docs.docker.com/get-docker/"
    echo ""
    echo "方式 2: 安装 ESP-IDF v5.1+"
    echo "  mkdir -p ~/esp && cd ~/esp"
    echo "  git clone --recursive https://github.com/espressif/esp-idf.git"
    echo "  cd esp-idf && ./install.sh esp32s3"
    echo "  . ./export.sh"
    echo ""
    echo "方式 3: 使用 Web IDE (无需安装)"
    echo "  访问 https://espressif.github.io/idf-eclipse-plugin/"
    echo ""
    echo "编译完成后, bin 文件位于:"
    echo "  build/bio_growth_recorder.bin           (主固件)"
    echo "  build/bootloader/bootloader.bin  (引导加载器)"
    echo "  build/partition_table/partition-table.bin  (分区表)"
    exit 1
fi
