@echo off
chcp 65001 >nul 2>&1
REM ============================================================
REM  ESP32 固件编译脚本 (Windows) - 生成 bin 文件
REM  使用 ESP-IDF Docker 镜像编译
REM  作者: Andrew 亚生
REM ============================================================

setlocal enabledelayedexpansion
set FIRMWARE_DIR=%~dp0
set OUTPUT_DIR=%FIRMWARE_DIR%build_output

echo ========================================================
echo   ESP32-S3 CAM 固件编译 (Windows)
echo   目标: 生成可烧录的 bin 文件
echo ========================================================
echo.

REM 检查 Docker
where docker >nul 2>&1
if %errorlevel% equ 0 (
    echo [1/4] 检测到 Docker, 使用 Docker 编译...
    echo.

    mkdir "%OUTPUT_DIR%" 2>nul

    echo [2/4] 正在拉取 ESP-IDF Docker 镜像...
    docker pull espressif/idf:v5.1.2

    echo [3/4] 正在编译固件...
    docker run --rm ^
        -v "%FIRMWARE_DIR%:/project" ^
        -v "%OUTPUT_DIR%:/output" ^
        -w /project ^
        espressif/idf:v5.1.2 ^
        bash -c ". /opt/esp/idf/export.sh && idf.py set-target esp32s3 && idf.py build && cp build/bio_growth_recorder.bin /output/bio-recorder-v3.0-esp32s3-firmware.bin && cp build/bootloader/bootloader.bin /output/bio-recorder-v3.0-esp32s3-bootloader.bin && cp build/partition_table/partition-table.bin /output/bio-recorder-v3.0-esp32s3-partitions.bin && echo BUILD_SUCCESS"

    echo.
    echo ========================================================
    echo   编译完成!
    echo   bin 文件位于: %OUTPUT_DIR%
    echo ========================================================
    dir "%OUTPUT_DIR%"
    echo.
    echo 烧录命令 (需要 esptool.py):
    echo   esptool.py --chip esp32s3 --port COM3 --baud 921600 ^
    echo     write_flash 0x0 bio-recorder-v3.0-esp32s3-bootloader.bin ^
    echo     0x8000 bio-recorder-v3.0-esp32s3-partitions.bin ^
    echo     0x10000 bio-recorder-v3.0-esp32s3-firmware.bin

) else (
    echo 错误: 未检测到 Docker
    echo.
    echo 请选择以下方式之一:
    echo.
    echo 方式 1: 安装 Docker Desktop for Windows
    echo   https://docs.docker.com/desktop/install/windows-install/
    echo.
    echo 方式 2: 安装 ESP-IDF v5.1+ for Windows
    echo   下载: https://dl.espressif.com/dl/esp-idf/
    echo   安装后打开 "ESP-IDF PowerShell" 或 "ESP-IDF CMD"
    echo   然后运行:
    echo     cd %FIRMWARE_DIR%
    echo     idf.py set-target esp32s3
    echo     idf.py build
    echo.
    echo 方式 3: 使用 ESP-IDF Web 在线编译
    echo   https://espressif.github.io/idf-eclipse-plugin/
)

pause
