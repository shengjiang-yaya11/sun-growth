#!/bin/bash
# ============================================================
#  ESP32 固件烧录脚本
#  使用 esptool.py 将 bin 文件烧录到 ESP32-S3 CAM
#  作者: Andrew 亚生
# ============================================================
set -e

echo "========================================================"
echo "  ESP32-S3 CAM 固件烧录"
echo "========================================================"
echo ""

# 检查 esptool.py
if ! command -v esptool.py &> /dev/null; then
    echo "错误: 未找到 esptool.py"
    echo "安装: pip install esptool"
    exit 1
fi

# 检查 bin 文件
BIN_DIR="$(cd "$(dirname "$0")" && pwd)/build_output"
BOOTLOADER="$BIN_DIR/bio-recorder-esp32s3-bootloader.bin"
PARTITIONS="$BIN_DIR/bio-recorder-esp32s3-partitions.bin"
FIRMWARE="$BIN_DIR/bio-recorder-esp32s3-firmware.bin"

if [ ! -f "$FIRMWARE" ]; then
    echo "错误: 未找到 bin 文件"
    echo "请先运行 ./build_firmware.sh 编译固件"
    exit 1
fi

# 检测串口
PORT="${1:-}"
if [ -z "$PORT" ]; then
    echo "检测串口设备..."
    if [ -e "/dev/ttyUSB0" ]; then
        PORT="/dev/ttyUSB0"
    elif [ -e "/dev/ttyACM0" ]; then
        PORT="/dev/ttyACM0"
    elif [ -e "/dev/cu.SLAB_USBtoUART" ]; then
        PORT="/dev/cu.SLAB_USBtoUART"
    else
        echo "未检测到串口设备, 请手动指定:"
        echo "  $0 /dev/ttyUSB0"
        echo "  $0 COM3  (Windows)"
        exit 1
    fi
fi

echo "串口: $PORT"
echo ""

# 烧录
echo "开始烧录..."
echo "  引导加载器: 0x0"
echo "  分区表:     0x8000"
echo "  主固件:     0x10000"
echo ""

esptool.py --chip esp32s3 --port "$PORT" --baud 921600 \
    write_flash \
    0x0 "$BOOTLOADER" \
    0x8000 "$PARTITIONS" \
    0x10000 "$FIRMWARE"

echo ""
echo "========================================================"
echo "  烧录完成!"
echo "========================================================"
echo ""
echo "下一步:"
echo "  1. 手机/电脑连接 WiFi: BioRecorder-XXXX"
echo "  2. 浏览器访问: http://192.168.4.1"
echo "  3. 填写 WiFi、服务器地址、设备ID、密钥"
echo "  4. 保存并重启"
echo ""
echo "查看串口日志:"
echo "  esptool.py --port $PORT monitor"
echo "  或 idf.py -p $PORT monitor"
