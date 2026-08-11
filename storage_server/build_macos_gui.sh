#!/bin/bash
# ============================================================
#  快速编译 macOS GUI 版本
#  需要: Xcode Command Line Tools (xcode-select --install)
# ============================================================
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

echo "========================================================"
echo "  编译 macOS GUI 版本"
echo "========================================================"
echo ""

# 检查 Go
if ! command -v go &> /dev/null; then
    echo "❌ 未找到 Go. 请安装: brew install go"
    exit 1
fi

# 检查 macOS
if [[ "$(uname)" != "Darwin" ]]; then
    echo "⚠ 此脚本仅适用于 macOS"
    echo "请使用 build_all.sh 编译其他平台"
    exit 1
fi

# 下载依赖
echo "[1/3] 下载 Go 依赖..."
go mod download

# 编译
ARCH=$(uname -m)
echo "[2/3] 编译 ($ARCH)..."
CGO_ENABLED=1 go build -o "dist/bio-recorder-darwin-$ARCH" .

echo "[3/3] 完成!"
echo ""
echo "产物: dist/bio-recorder-darwin-$ARCH"
ls -lh "dist/bio-recorder-darwin-$ARCH"

echo ""
echo "=== 下一步 ==="
echo ""
echo "1. 解除隔离 (macOS Gatekeeper):"
echo "   xattr -cr dist/bio-recorder-darwin-$ARCH"
echo ""
echo "2. 创建配置文件:"
echo "   cp config.example.json config.json"
echo ""
echo "3. 运行:"
echo "   ./dist/bio-recorder-darwin-$ARCH"
echo ""
echo "   或 headless 模式 (命令行):"
echo "   ./dist/bio-recorder-darwin-$ARCH --headless"
