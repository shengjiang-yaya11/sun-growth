#!/bin/bash
# ============================================================
#  生物成长记录仪 - 全平台编译脚本 (v3.0)
#  作者: Andrew 亚生
# ============================================================
#  macOS: 需要 Xcode Command Line Tools (xcode-select --install)
#  Windows 交叉编译: 需要 mingw-w64 + gcc
# ============================================================
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
OUTPUT_DIR="$SCRIPT_DIR/dist"
mkdir -p "$OUTPUT_DIR"

echo "========================================================"
echo "  生物成长记录仪 v3.0 - 全平台编译"
echo "========================================================"
echo ""

# 检查 Go 版本
GO_VERSION=$(go version | awk '{print $3}')
echo "Go 版本: $GO_VERSION"
echo "输出目录: $OUTPUT_DIR"
echo ""

# ==================== macOS ====================
echo "--- macOS (ARM64 + AMD64) ---"

if [[ "$(uname)" == "Darwin" ]]; then
    # macOS ARM64 (Apple Silicon) GUI 版
    echo "  编译 macOS ARM64 GUI..."
    CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 \
        go build -o "$OUTPUT_DIR/bio-recorder-darwin-arm64" .
    echo "  ✔ bio-recorder-darwin-arm64"

    # macOS AMD64 (Intel) GUI 版
    echo "  编译 macOS AMD64 GUI..."
    CGO_ENABLED=1 GOOS=darwin GOARCH=amd64 \
        go build -o "$OUTPUT_DIR/bio-recorder-darwin-amd64" .
    echo "  ✔ bio-recorder-darwin-amd64"
else
    echo "  ⚠ 跳过 macOS 编译 (非 macOS 环境)"
fi

# ==================== Linux ====================
echo "--- Linux (AMD64 + ARM64) ---"

if [[ "$(uname)" == "Darwin" || "$(uname)" == "Linux" ]]; then
    # Linux AMD64 GUI 版
    echo "  编译 Linux AMD64 GUI..."
    CGO_ENABLED=1 GOOS=linux GOARCH=amd64 \
        go build -o "$OUTPUT_DIR/bio-recorder-linux-gui-amd64" .
    echo "  ✔ bio-recorder-linux-gui-amd64"

    # Linux AMD64 Headless 版 (NAS/Docker)
    echo "  编译 Linux AMD64 Headless (CGO 禁用)..."
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
        go build -tags headless -o "$OUTPUT_DIR/bio-recorder-nas-amd64" .
    echo "  ✔ bio-recorder-nas-amd64"

    # Linux ARM64 Headless 版 (树莓派/NAS)
    echo "  编译 Linux ARM64 Headless (CGO 禁用)..."
    CGO_ENABLED=0 GOOS=linux GOARCH=arm64 \
        go build -tags headless -o "$OUTPUT_DIR/bio-recorder-nas-arm64" .
    echo "  ✔ bio-recorder-nas-arm64"
else
    echo "  ⚠ 跳过 Linux 编译"
fi

# ==================== Windows ====================
echo "--- Windows (AMD64) ---"

if [[ "$(uname)" == "Darwin" || "$(uname)" == "Linux" ]]; then
    # Windows AMD64 GUI 版 (需要 mingw-w64)
    if command -v x86_64-w64-mingw32-gcc &> /dev/null || command -v x86_64-w64-mingw32-gcc-posix &> /dev/null; then
        echo "  编译 Windows AMD64 GUI..."
        # 设置 CC 为 mingw 交叉编译器
        export CC=x86_64-w64-mingw32-gcc
        if command -v x86_64-w64-mingw32-gcc-posix &> /dev/null; then
            export CC=x86_64-w64-mingw32-gcc-posix
        fi
        CGO_ENABLED=1 GOOS=windows GOARCH=amd64 \
            go build -o "$OUTPUT_DIR/bio-recorder-windows-gui.exe" .
        echo "  ✔ bio-recorder-windows-gui.exe"
    else
        echo "  ⚠ 跳过 Windows GUI (需要 mingw-w64, 安装: brew install mingw-w64)"
    fi

    # Windows Headless 版 (不需要 mingw)
    echo "  编译 Windows Headless (CGO 禁用)..."
    CGO_ENABLED=0 GOOS=windows GOARCH=amd64 \
        go build -tags headless -o "$OUTPUT_DIR/bio-recorder-windows-headless.exe" .
    echo "  ✔ bio-recorder-windows-headless.exe"
else
    echo "  ⚠ 跳过 Windows 编译"
fi

echo ""
echo "========================================================"
echo "  编译完成!"
echo "========================================================"
echo ""
echo "产物列表:"
ls -lh "$OUTPUT_DIR/"
echo ""
echo "=== 使用方法 ==="
echo ""
echo "macOS GUI:"
echo "  ./bio-recorder-darwin-arm64     (Apple Silicon)"
echo "  ./bio-recorder-darwin-amd64     (Intel)"
echo "  首次运行前: xattr -cr bio-recorder-darwin-*"
echo ""
echo "Windows GUI:"
echo "  bio-recorder-windows-gui.exe"
echo "  需要: VC++ Redistributable + 支持 OpenGL 3.2 的显卡驱动"
echo ""
echo "Headless (所有平台, 命令行模式):"
echo "  ./bio-recorder-server --headless"
echo "  浏览器打开 http://localhost:8443"
echo ""
echo "Docker:"
echo "  docker compose up -d"
