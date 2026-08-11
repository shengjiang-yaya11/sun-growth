// ============================================================
//  【电脑端软件】Headless 模式 (NAS / 无显示环境)
//  编译条件: CGO_ENABLED=0 (无 C 编译器或无 OpenGL)
//  自动回退到命令行模式
// ============================================================
//
//go:build !cgo

package main

import (
	"fmt"
	"runtime"
)

// runGUI 在无 CGO 环境下回退到 headless 模式
func runGUI(serverApp *ServerApp) {
	fmt.Println()
	fmt.Println("========================================================")
	fmt.Println("  ⚠ GUI 模式不可用 (CGO 编译环境缺失)")
	fmt.Println("========================================================")
	fmt.Println()
	fmt.Println("当前环境:", runtime.GOOS, "/", runtime.GOARCH)
	fmt.Println()
	fmt.Println("GUI 需要: CGO_ENABLED=1 + 以下依赖")

	switch runtime.GOOS {
	case "darwin":
		fmt.Println("  macOS: Xcode Command Line Tools")
		fmt.Println("  安装:  xcode-select --install")
		fmt.Println("  编译:  CGO_ENABLED=1 go build -o bio-recorder .")
	case "windows":
		fmt.Println("  Windows: MSYS2/MinGW-w64 + OpenGL 驱动")
		fmt.Println("  编译:  set CGO_ENABLED=1 && go build -o bio-recorder.exe .")
	case "linux":
		fmt.Println("  Linux: build-essential + libgl1-mesa-dev + xorg-dev")
		fmt.Println("  安装:  sudo apt install build-essential libgl1-mesa-dev xorg-dev")
	default:
		fmt.Println("  请安装对应平台的 C 编译器和 OpenGL 开发库")
	}
	fmt.Println()
	fmt.Println("回退到 Headless (命令行) 模式...")
	fmt.Println("浏览器访问 http://localhost:8443 可完全替代 GUI")
	fmt.Println("========================================================")
	fmt.Println()
	runHeadless(serverApp)
}
