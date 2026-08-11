// ============================================================
//  【电脑端软件】Headless 命令行模式
//  适用场景: NAS / Docker / 服务器 / 无显示环境
//  运行方式: ./bio-recorder-server --headless [config.json]
// ============================================================

package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

// runHeadless 以命令行模式运行服务器
func runHeadless(serverApp *ServerApp) {
	serverApp.PrintBanner()

	if err := serverApp.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "服务器启动失败: %v\n", err)
		os.Exit(1)
	}

	// 等待退出信号
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	fmt.Printf("\n收到信号 %s, 正在关闭...\n", sig.String())
	serverApp.Shutdown()
}
