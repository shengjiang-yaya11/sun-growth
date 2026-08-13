// ============================================================
//  【电脑端软件】GUI 窗口模式 (Windows / macOS / Linux)
//  编译条件: 需要 CGO (CGO_ENABLED=1) 和 C 编译器
//  无 CGO 环境下自动回退到 headless 模式 (gui_stub.go)
// ============================================================
//
// 功能:
//   - 单按钮启动/停止服务器切换 (不自动启动, 先配置参数)
//   - 6 种语言实时切换 (英语/简中/繁中/日语/西语/法语)
//   - 仪表盘 / 设置 / 日志 三个标签页
//   - ESP32 设置向导 (服务器地址复制 + 设备连接检查)
//   - ESP32 设备状态实时显示
//   - 参数调试和保存文件夹配置
//
//go:build cgo

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"bio-growth-recorder/config"
	"bio-growth-recorder/i18n"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// ============================================================
// 日志捕获: 自定义 slog Handler, 同时输出到 stdout 和 GUI
// ============================================================

// LogCapture 日志捕获器, 实现 slog.Handler 接口
type LogCapture struct {
	mu       sync.Mutex
	lines    []string
	maxLine  int
	handler  slog.Handler
	onChange func(lines []string)
}

// NewLogCapture 创建日志捕获器
// onChange 回调接收日志行的拷贝, 在锁外调用, 避免死锁
func NewLogCapture(onChange func(lines []string)) *LogCapture {
	return &LogCapture{
		maxLine:  500,
		handler:  slog.NewJSONHandler(os.Stdout, nil),
		onChange: onChange,
	}
}

func (lc *LogCapture) Enabled(ctx context.Context, level slog.Level) bool {
	return true
}

func (lc *LogCapture) Handle(ctx context.Context, r slog.Record) error {
	// 输出到 stdout
	_ = lc.handler.Handle(ctx, r)

	msg := fmt.Sprintf("[%s] %s", r.Level.String(), r.Message)
	r.Attrs(func(a slog.Attr) bool {
		msg += fmt.Sprintf(" %s=%v", a.Key, a.Value)
		return true
	})

	// 加锁追加日志行, 拷贝后释放锁, 再回调 (避免死锁)
	lc.mu.Lock()
	lc.lines = append(lc.lines, msg)
	if len(lc.lines) > lc.maxLine {
		lc.lines = lc.lines[len(lc.lines)-lc.maxLine:]
	}
	linesCopy := make([]string, len(lc.lines))
	copy(linesCopy, lc.lines)
	lc.mu.Unlock()

	// 在锁外回调, 避免 onChange 内部再次加锁导致死锁
	if lc.onChange != nil {
		lc.onChange(linesCopy)
	}
	return nil
}

func (lc *LogCapture) WithAttrs(attrs []slog.Attr) slog.Handler {
	return lc
}

func (lc *LogCapture) WithGroup(name string) slog.Handler {
	return lc
}

// GetLines 获取已捕获的日志行
func (lc *LogCapture) GetLines() []string {
	lc.mu.Lock()
	defer lc.mu.Unlock()
	result := make([]string, len(lc.lines))
	copy(result, lc.lines)
	return result
}

// Clear 清空日志
func (lc *LogCapture) Clear() {
	lc.mu.Lock()
	defer lc.mu.Unlock()
	lc.lines = nil
}

// ============================================================
// GUI 主入口
// ============================================================

// guiState GUI 全局状态
type guiState struct {
	serverApp  *ServerApp
	fyneApp    fyne.App
	window     fyne.Window
	logCapture *LogCapture
	logEntry   *widget.Entry

	// 当前语言
	currentLang string

	// 仪表盘组件
	statusLabel      *widget.Label
	urlLabel         *widget.Label
	uptimeLabel      *widget.Label
	photoLabel       *widget.Label
	sizeLabel        *widget.Label
	deviceTable      *widget.Table
	devices          []deviceStatusInfo
	toggleBtn        *widget.Button // 单按钮: 启动/停止切换
	galleryBtn       *widget.Button
	statusCard       *widget.Card
	deviceCard       *widget.Card
	esp32Card        *widget.Card
	esp32URLLabel    *widget.Label
	esp32StatusLabel *widget.Label

	// v3.1: ESP32 双向控制
	esp32IPEntry     *widget.Entry
	esp32SecretEntry *widget.Entry
	esp32ScanBtn     *widget.Button
	esp32CaptureBtn  *widget.Button
	esp32ScheduleBtn *widget.Button
	esp32StatusBtn   *widget.Button
	esp32DelayEntry  *widget.Entry
	esp32RSSILabel   *widget.Label
	esp32HeapLabel   *widget.Label

	// 语言选择器
	langSelect *widget.Select

	// 标签页
	tabs *container.AppTabs

	// 仪表盘滚动容器
	dashContainer *container.Scroll

	// 设置页组件 (用于语言切换时更新)
	settingsContent *container.Scroll

	mu sync.Mutex
}

// deviceStatusInfo 设备状态信息 (从 API 获取)
type deviceStatusInfo struct {
	DeviceID        string `json:"device_id"`
	DeviceName      string `json:"device_name"`
	Status          string `json:"status"`
	FirmwareVersion string `json:"firmware_version"`
	LastSeen        string `json:"last_seen"`
	PhotoCount      int64  `json:"photo_count"`
	PhotoInterval   int    `json:"photo_interval"`
}

// runGUI 启动 GUI 窗口模式
func runGUI(serverApp *ServerApp) {
	// 1. 创建 Fyne 应用 (使用唯一 ID 以支持偏好设置存储)
	fyneApp := app.NewWithID("com.andrew.bio-growth-recorder")

	gs := &guiState{
		serverApp:   serverApp,
		fyneApp:     fyneApp,
		currentLang: serverApp.Cfg.DefaultLang,
	}

	window := fyneApp.NewWindow(i18n.Translate(gs.currentLang, i18n.GuiTitle))
	window.Resize(fyne.NewSize(900, 650))
	window.SetMaster()
	gs.window = window

	// 2. 创建日志捕获器 (修复死锁: onChange 接收 lines 拷贝, 在锁外调用)
	var logEntry *widget.Entry
	gs.logCapture = NewLogCapture(func(lines []string) {
		if logEntry != nil {
			logEntry.SetText(strings.Join(lines, "\n"))
		}
	})
	gs.logEntry = logEntry
	slog.SetDefault(slog.New(gs.logCapture))

	// 3. 创建 GUI 组件 (不自动启动服务器)
	gs.dashContainer = gs.buildDashboard()
	gs.settingsContent = gs.buildSettings()
	logsTab := gs.buildLogsTab(&logEntry)
	gs.logEntry = logEntry

	// 4. 组装标签页
	gs.tabs = container.NewAppTabs(
		container.NewTabItemWithIcon(
			i18n.Translate(gs.currentLang, i18n.GuiDashboard),
			theme.HomeIcon(),
			gs.dashContainer,
		),
		container.NewTabItemWithIcon(
			i18n.Translate(gs.currentLang, i18n.GuiSettings),
			theme.SettingsIcon(),
			gs.settingsContent,
		),
		container.NewTabItemWithIcon(
			i18n.Translate(gs.currentLang, i18n.GuiLogs),
			theme.DocumentIcon(),
			logsTab,
		),
	)
	gs.tabs.SetTabLocation(container.TabLocationTop)

	window.SetContent(gs.tabs)

	// 5. 窗口关闭时优雅关闭服务器
	window.SetCloseIntercept(func() {
		slog.Info("正在关闭应用...")
		gs.serverApp.Shutdown()
		window.Close()
	})

	// 6. 更新初始 UI 状态 (服务器未启动, 按钮显示"启动服务器")
	gs.updateToggleButton()

	// 7. 运行 GUI (阻塞)
	window.ShowAndRun()
}

// ============================================================
// 仪表盘标签页
// ============================================================

// buildDashboard 创建仪表盘 UI
func (gs *guiState) buildDashboard() *container.Scroll {
	lang := gs.currentLang

	gs.statusLabel = widget.NewLabel(i18n.Translate(lang, i18n.GuiStopped))
	gs.statusLabel.TextStyle = fyne.TextStyle{Bold: true}

	gs.urlLabel = widget.NewLabel(gs.serverApp.ServerURL())
	gs.uptimeLabel = widget.NewLabel("00:00:00")
	gs.photoLabel = widget.NewLabel("0")
	gs.sizeLabel = widget.NewLabel("0 MB")

	// 创建设备状态表格
	gs.deviceTable = widget.NewTable(
		func() (int, int) {
			gs.mu.Lock()
			defer gs.mu.Unlock()
			return len(gs.devices) + 1, 6 // +1 for header
		},
		func() fyne.CanvasObject {
			return widget.NewLabel("Cell")
		},
		func(id widget.TableCellID, obj fyne.CanvasObject) {
			label := obj.(*widget.Label)
			l := gs.currentLang
			if id.Row == 0 {
				headers := []string{
					i18n.Translate(l, i18n.DeviceID),
					i18n.Translate(l, i18n.GuiDeviceName),
					i18n.Translate(l, i18n.Status),
					i18n.Translate(l, i18n.LastSeen),
					i18n.Translate(l, i18n.GuiPhotoCount),
					i18n.Translate(l, i18n.GuiFirmwareVer),
				}
				if id.Col < len(headers) {
					label.SetText(headers[id.Col])
					label.TextStyle = fyne.TextStyle{Bold: true}
				}
				return
			}
			gs.mu.Lock()
			defer gs.mu.Unlock()
			idx := id.Row - 1
			if idx >= len(gs.devices) {
				label.SetText("")
				return
			}
			dev := gs.devices[idx]
			switch id.Col {
			case 0:
				label.SetText(dev.DeviceID)
			case 1:
				label.SetText(dev.DeviceName)
			case 2:
				if dev.Status == "active" {
					label.SetText(i18n.Translate(l, i18n.GuiOnline))
				} else {
					label.SetText(i18n.Translate(l, i18n.GuiOffline))
				}
			case 3:
				label.SetText(dev.LastSeen)
			case 4:
				label.SetText(fmt.Sprintf("%d", dev.PhotoCount))
			case 5:
				label.SetText(dev.FirmwareVersion)
			}
			label.TextStyle = fyne.TextStyle{}
		},
	)

	// 设置列宽
	gs.deviceTable.SetColumnWidth(0, 140)
	gs.deviceTable.SetColumnWidth(1, 100)
	gs.deviceTable.SetColumnWidth(2, 80)
	gs.deviceTable.SetColumnWidth(3, 180)
	gs.deviceTable.SetColumnWidth(4, 80)
	gs.deviceTable.SetColumnWidth(5, 100)

	// 单按钮: 启动/停止切换 (根据服务器状态自动切换文本和颜色)
	gs.toggleBtn = widget.NewButton(i18n.Translate(lang, i18n.GuiStartServer), func() {
		if gs.serverApp.Running {
			gs.stopServer()
		} else {
			gs.startServer()
		}
	})
	gs.toggleBtn.Importance = widget.HighImportance

	// 打开画廊按钮
	gs.galleryBtn = widget.NewButton(i18n.Translate(lang, i18n.GuiOpenGallery), func() {
		OpenBrowser(gs.serverApp.ServerURL())
	})
	gs.galleryBtn.Importance = widget.MediumImportance

	// 语言选择器
	langOptions := i18n.SupportedLanguages
	langNames := make([]string, len(langOptions))
	for i, l := range langOptions {
		langNames[i] = fmt.Sprintf("%s (%s)", i18n.LanguageNames[l], l)
	}
	gs.langSelect = widget.NewSelect(langNames, func(selected string) {
		for i, l := range langOptions {
			if langNames[i] == selected {
				gs.currentLang = l
				gs.serverApp.Cfg.DefaultLang = l
				gs.refreshUILanguage()
				break
			}
		}
	})
	for i, l := range langOptions {
		if l == gs.currentLang {
			gs.langSelect.SetSelectedIndex(i)
			break
		}
	}

	// 服务器状态卡片
	gs.statusCard = widget.NewCard(i18n.Translate(lang, i18n.GuiServerStatus), "", container.NewVBox(
		container.NewHBox(
			widget.NewLabel(i18n.Translate(lang, i18n.Status)+":"), gs.statusLabel,
		),
		container.NewHBox(
			widget.NewLabel(i18n.Translate(lang, i18n.GuiAccessURL)), gs.urlLabel,
		),
		container.NewHBox(
			widget.NewLabel(i18n.Translate(lang, i18n.GuiUptime)), gs.uptimeLabel,
		),
		container.NewHBox(
			widget.NewLabel(i18n.Translate(lang, i18n.GuiTotalPhotos)), gs.photoLabel,
		),
		container.NewHBox(
			widget.NewLabel(i18n.Translate(lang, i18n.GuiStorageUsed)), gs.sizeLabel,
		),
		container.NewHBox(
			widget.NewLabel(i18n.Translate(lang, i18n.GuiLanguage)), gs.langSelect,
		),
	))

	// 设备状态卡片
	gs.deviceCard = widget.NewCard(i18n.Translate(lang, i18n.GuiDeviceStatus), "", gs.deviceTable)

	// ESP32 设置向导卡片
	gs.esp32URLLabel = widget.NewLabel(gs.serverApp.ServerURL())
	gs.esp32URLLabel.TextStyle = fyne.TextStyle{Bold: true}
	gs.esp32URLLabel.Wrapping = fyne.TextWrapWord

	gs.esp32StatusLabel = widget.NewLabel(i18n.Translate(lang, i18n.GuiWaitingESP32))
	gs.esp32StatusLabel.TextStyle = fyne.TextStyle{Bold: true}

	copyURLBtn := widget.NewButton(i18n.Translate(lang, i18n.GuiCopyURL), func() {
		gs.fyneApp.Clipboard().SetContent(gs.serverApp.ServerURL())
		slog.Info(i18n.Translate(gs.currentLang, i18n.GuiURLCopied))
	})

	checkDevicesBtn := widget.NewButton(i18n.Translate(lang, i18n.GuiCheckDevices), func() {
		go gs.checkESP32Connection()
	})

	gs.esp32Card = widget.NewCard(i18n.Translate(lang, i18n.GuiESP32Setup), "", container.NewVBox(
		widget.NewLabel(i18n.Translate(lang, i18n.GuiESP32Step1)),
		widget.NewLabel(i18n.Translate(lang, i18n.GuiESP32Step2)),
		widget.NewLabel(i18n.Translate(lang, i18n.GuiESP32Step3)),
		container.NewHBox(
			widget.NewLabel(i18n.Translate(lang, i18n.GuiServerForESP32)),
			gs.esp32URLLabel,
			copyURLBtn,
		),
		widget.NewLabel(i18n.Translate(lang, i18n.GuiSoftAPHint)),
		gs.esp32StatusLabel,
		checkDevicesBtn,
	))

	// v3.1: ESP32 双向远程控制面板
	gs.esp32IPEntry = widget.NewEntry()
	gs.esp32IPEntry.SetPlaceHolder("ESP32 IP (192.168.x.x)")
	gs.esp32SecretEntry = widget.NewPasswordEntry()
	gs.esp32SecretEntry.SetPlaceHolder("设备密钥 (64位十六进制)")
	if devices, err := gs.serverApp.DB.ListDevices(); err == nil && len(devices) > 0 {
		gs.esp32SecretEntry.SetText(devices[0].SecretHex)
	}
	gs.esp32DelayEntry = widget.NewEntry()
	gs.esp32DelayEntry.SetPlaceHolder("30")

	doCapture := func() {
		ip := strings.TrimSpace(gs.esp32IPEntry.Text)
		secret := strings.TrimSpace(gs.esp32SecretEntry.Text)
		if ip == "" {
			dialog.ShowInformation(i18n.Translate(gs.currentLang, i18n.GuiCaptureSent),
				i18n.Translate(gs.currentLang, i18n.GuiIPRequired), gs.window)
			return
		}
		if secret == "" {
			dialog.ShowInformation(i18n.Translate(gs.currentLang, i18n.GuiCaptureSent),
				i18n.Translate(gs.currentLang, i18n.GuiSecretRequired), gs.window)
			return
		}
		if err := gs.serverApp.DevCtrl.CaptureNow(ip, 8081, secret); err != nil {
			dialog.ShowError(fmt.Errorf("%s: %v", i18n.Translate(gs.currentLang, i18n.GuiCaptureFailed), err), gs.window)
		} else {
			dialog.ShowInformation(i18n.Translate(gs.currentLang, i18n.GuiCaptureSent),
				i18n.Translate(gs.currentLang, i18n.GuiCaptureQueued), gs.window)
		}
	}

	gs.esp32CaptureBtn = widget.NewButton(i18n.Translate(lang, i18n.GuiRemoteCapture), func() {
		doCapture()
	})
	gs.esp32CaptureBtn.Disable()

	gs.esp32ScheduleBtn = widget.NewButton(i18n.Translate(lang, i18n.GuiCaptureOnce), func() {
		ip := strings.TrimSpace(gs.esp32IPEntry.Text)
		secret := strings.TrimSpace(gs.esp32SecretEntry.Text)
		if ip == "" {
			dialog.ShowInformation(i18n.Translate(gs.currentLang, i18n.GuiCaptureSent),
				i18n.Translate(gs.currentLang, i18n.GuiIPRequired), gs.window)
			return
		}
		if secret == "" {
			dialog.ShowInformation(i18n.Translate(gs.currentLang, i18n.GuiCaptureSent),
				i18n.Translate(gs.currentLang, i18n.GuiSecretRequired), gs.window)
			return
		}
		delaySec, err := strconv.Atoi(strings.TrimSpace(gs.esp32DelayEntry.Text))
		if err != nil || delaySec < 1 || delaySec > 86400 {
			dialog.ShowInformation(i18n.Translate(gs.currentLang, i18n.GuiScheduleCapture),
				i18n.Translate(gs.currentLang, i18n.GuiScheduleInvalid), gs.window)
			return
		}
		gs.esp32ScheduleBtn.Disable()
		go func() {
			time.Sleep(time.Duration(delaySec) * time.Second)
			gs.esp32ScheduleBtn.Enable()
			doCapture()
		}()
		dialog.ShowInformation(i18n.Translate(gs.currentLang, i18n.GuiScheduled),
			fmt.Sprintf("%s: %d s", i18n.Translate(gs.currentLang, i18n.GuiScheduleCapture), delaySec), gs.window)
	})
	gs.esp32ScheduleBtn.Disable()

	gs.esp32StatusBtn = widget.NewButton(i18n.Translate(lang, i18n.GuiRefreshStatus), func() {
		ip := strings.TrimSpace(gs.esp32IPEntry.Text)
		if ip == "" {
			return
		}
		status, err := gs.serverApp.DevCtrl.GetDeviceStatus(ip, 8081)
		if err != nil {
			gs.esp32RSSILabel.SetText(i18n.Translate(gs.currentLang, i18n.GuiOfflineUnreachable))
			gs.esp32HeapLabel.SetText("")
			return
		}
		gs.esp32RSSILabel.SetText(fmt.Sprintf(i18n.Translate(gs.currentLang, i18n.GuiSignal),
			status.WiFiRSSI, status.UptimeSec, status.FWVersion))
		gs.esp32HeapLabel.SetText(fmt.Sprintf(i18n.Translate(gs.currentLang, i18n.GuiFreeHeap), status.FreeHeap/1024))
		gs.esp32CaptureBtn.Enable()
		gs.esp32ScheduleBtn.Enable()
	})
	gs.esp32StatusBtn.Disable()

	gs.esp32RSSILabel = widget.NewLabel(i18n.Translate(lang, i18n.GuiNotConnected))
	gs.esp32HeapLabel = widget.NewLabel("")

	gs.esp32ScanBtn = widget.NewButton(i18n.Translate(lang, i18n.GuiScanDevices), func() {
		gs.esp32ScanBtn.Disable()
		gs.esp32ScanBtn.SetText(i18n.Translate(gs.currentLang, i18n.GuiDiscovering))
		go func() {
			devices := gs.serverApp.DevCtrl.DiscoverDevices()
			if len(devices) > 0 {
				gs.esp32IPEntry.SetText(devices[0].IP)
				gs.esp32StatusBtn.Enable()
				gs.esp32CaptureBtn.Enable()
				gs.esp32ScheduleBtn.Enable()
				dialog.ShowInformation(i18n.Translate(gs.currentLang, i18n.GuiESP32Control),
					fmt.Sprintf(i18n.Translate(gs.currentLang, i18n.GuiDevicesFound), len(devices), devices[0].IP), gs.window)
			} else {
				dialog.ShowInformation(i18n.Translate(gs.currentLang, i18n.GuiESP32Control),
					i18n.Translate(gs.currentLang, i18n.GuiNoDevicesFound), gs.window)
			}
			gs.esp32ScanBtn.SetText(i18n.Translate(gs.currentLang, i18n.GuiScanDevices))
			gs.esp32ScanBtn.Enable()
		}()
	})

	esp32ControlCard := widget.NewCard(i18n.Translate(lang, i18n.GuiESP32Control), "",
		container.NewVBox(
			gs.esp32ScanBtn,
			widget.NewLabel(i18n.Translate(lang, i18n.GuiDeviceIP)),
			gs.esp32IPEntry,
			widget.NewLabel(i18n.Translate(lang, i18n.GuiDeviceSecretLabel)),
			gs.esp32SecretEntry,
			container.NewHBox(gs.esp32CaptureBtn, gs.esp32ScheduleBtn, gs.esp32StatusBtn),
			container.NewHBox(widget.NewLabel(i18n.Translate(lang, i18n.GuiDelaySeconds)), gs.esp32DelayEntry),
			gs.esp32RSSILabel,
			gs.esp32HeapLabel,
		),
	)

	// 操作按钮行 (单按钮切换 + 画廊)
	buttonRow := container.NewHBox(gs.toggleBtn, gs.galleryBtn)

	// 组装
	content := container.NewVBox(
		esp32ControlCard,
		gs.statusCard,
		buttonRow,
		gs.esp32Card,
		gs.deviceCard,
	)

	return container.NewVScroll(content)
}

// startServer 启动服务器
func (gs *guiState) startServer() {
	lang := gs.currentLang
	gs.statusLabel.SetText(i18n.Translate(lang, i18n.GuiStarting))
	gs.updateToggleButton()

	go func() {
		slog.Info("正在启动服务器...")
		if err := gs.serverApp.Start(); err != nil {
			slog.Error("服务器启动失败", "error", err)
			fyne.CurrentApp().SendNotification(&fyne.Notification{
				Title:   i18n.Translate(lang, i18n.GuiStartFailed),
				Content: err.Error(),
			})
			gs.window.RequestFocus()
			dialog.ShowError(fmt.Errorf("%s: %v", i18n.Translate(lang, i18n.GuiStartFailed), err), gs.window)
			gs.statusLabel.SetText(i18n.Translate(lang, i18n.GuiStopped))
			gs.updateToggleButton()
			return
		}
		gs.serverApp.PrintBanner()
		slog.Info("服务器已启动", "url", gs.serverApp.ServerURL())

		gs.statusLabel.SetText(i18n.Translate(lang, i18n.GuiRunning))
		gs.urlLabel.SetText(gs.serverApp.ServerURL())
		gs.esp32URLLabel.SetText(gs.serverApp.ServerURL())
		gs.updateToggleButton()

		// 启动设备状态轮询
		go gs.pollDeviceStatus()
	}()
}

// stopServer 停止服务器
func (gs *guiState) stopServer() {
	lang := gs.currentLang
	gs.statusLabel.SetText(i18n.Translate(lang, i18n.GuiStopping))
	gs.updateToggleButton()

	go func() {
		slog.Info("正在停止服务器...")
		gs.serverApp.Stop()
		slog.Info("服务器已停止")

		gs.statusLabel.SetText(i18n.Translate(lang, i18n.GuiStopped))
		gs.uptimeLabel.SetText("00:00:00")
		gs.updateToggleButton()
	}()
}

// updateToggleButton 根据服务器运行状态更新单按钮文本、颜色和可用性
func (gs *guiState) updateToggleButton() {
	lang := gs.currentLang
	if gs.toggleBtn != nil {
		if gs.serverApp.Running {
			gs.toggleBtn.SetText(i18n.Translate(lang, i18n.GuiStopServer))
			gs.toggleBtn.Importance = widget.DangerImportance
		} else {
			gs.toggleBtn.SetText(i18n.Translate(lang, i18n.GuiStartServer))
			gs.toggleBtn.Importance = widget.HighImportance
		}
		gs.toggleBtn.Refresh()
	}
	if gs.galleryBtn != nil {
		if gs.serverApp.Running {
			gs.galleryBtn.Enable()
		} else {
			gs.galleryBtn.Disable()
		}
	}
}

// checkESP32Connection 检查 ESP32 设备连接状态
func (gs *guiState) checkESP32Connection() {
	lang := gs.currentLang
	gs.esp32StatusLabel.SetText(i18n.Translate(lang, i18n.GuiCheckDevices) + "...")

	devices, err := gs.serverApp.DB.ListDevices()
	if err != nil {
		slog.Warn("检查设备失败", "error", err)
		gs.esp32StatusLabel.SetText(i18n.Translate(lang, i18n.GuiNoDevicesYet))
		return
	}

	if len(devices) == 0 {
		gs.esp32StatusLabel.SetText(i18n.Translate(lang, i18n.GuiNoDevicesYet))
	} else {
		// 统计在线设备
		onlineCount := 0
		for _, dev := range devices {
			if !dev.LastSeen.IsZero() && time.Since(dev.LastSeen) < 5*time.Minute {
				onlineCount++
			}
		}
		if onlineCount > 0 {
			gs.esp32StatusLabel.SetText(fmt.Sprintf("%d/%d %s",
				onlineCount, len(devices), i18n.Translate(lang, i18n.GuiOnline)))
		} else {
			gs.esp32StatusLabel.SetText(fmt.Sprintf("%d %s, %s",
				len(devices), i18n.Translate(lang, i18n.GuiOffline),
				i18n.Translate(lang, i18n.GuiWaitingESP32)))
		}
	}

	// 同时更新仪表盘
	if gs.serverApp.Running {
		gs.updateDashboard()
	}
}

// pollDeviceStatus 轮询设备状态 (后台协程)
func (gs *guiState) pollDeviceStatus() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	// 立即执行一次
	gs.updateDashboard()

	for range ticker.C {
		if !gs.serverApp.Running {
			return
		}
		gs.updateDashboard()
	}
}

// updateDashboard 更新仪表盘数据
func (gs *guiState) updateDashboard() {
	if !gs.serverApp.Running {
		return
	}

	baseURL := fmt.Sprintf("http://127.0.0.1:%d", gs.serverApp.Cfg.Port)

	// 获取统计信息
	resp, err := http.Get(baseURL + "/health")
	if err != nil {
		slog.Warn("仪表盘: 获取健康状态失败", "error", err)
		return
	}
	defer resp.Body.Close()

	var health struct {
		Status  string `json:"status"`
		Uptime  int64  `json:"uptime"`
		Devices int64  `json:"devices"`
		Photos  int64  `json:"photos"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		return
	}

	// 更新 UI
	uptime := time.Duration(health.Uptime) * time.Second
	uptimeStr := fmt.Sprintf("%02d:%02d:%02d",
		int(uptime.Hours()),
		int(uptime.Minutes())%60,
		int(uptime.Seconds())%60,
	)

	gs.uptimeLabel.SetText(uptimeStr)
	gs.photoLabel.SetText(fmt.Sprintf("%d", health.Photos))

	// 获取设备列表 (直接从数据库查询)
	devices, err := gs.serverApp.DB.ListDevices()
	if err != nil {
		slog.Warn("仪表盘: 获取设备列表失败", "error", err)
		return
	}

	lang := gs.currentLang
	gs.mu.Lock()
	gs.devices = make([]deviceStatusInfo, 0, len(devices))
	for _, dev := range devices {
		count, _ := gs.serverApp.DB.CountPhotosByDevice(dev.DeviceID)
		lastSeen := i18n.Translate(lang, i18n.GuiNever)
		if !dev.LastSeen.IsZero() {
			lastSeen = dev.LastSeen.Format("2006-01-02 15:04:05")
			// 判断在线状态: 5 分钟内有活动 = 在线
			if time.Since(dev.LastSeen) < 5*time.Minute {
				dev.Status = "active"
			} else {
				dev.Status = "inactive"
			}
		}
		fwVer := dev.FirmwareVersion
		if fwVer == "" {
			fwVer = i18n.Translate(lang, i18n.GuiUnknown)
		}
		gs.devices = append(gs.devices, deviceStatusInfo{
			DeviceID:        dev.DeviceID,
			DeviceName:      dev.DeviceName,
			Status:          dev.Status,
			FirmwareVersion: fwVer,
			LastSeen:        lastSeen,
			PhotoCount:      count,
			PhotoInterval:   dev.PhotoInterval,
		})
	}
	gs.mu.Unlock()

	gs.deviceTable.Refresh()

	// 获取存储统计
	totalPhotos, totalSize, _ := gs.serverApp.Store.GetStats()
	gs.photoLabel.SetText(fmt.Sprintf("%d", totalPhotos))
	gs.sizeLabel.SetText(fmt.Sprintf("%d MB", totalSize/(1024*1024)))
}

// ============================================================
// 设置标签页
// ============================================================

// buildSettings 创建设置标签页
func (gs *guiState) buildSettings() *container.Scroll {
	cfg := gs.serverApp.Cfg
	lang := gs.currentLang
	window := gs.window

	// 端口
	portEntry := widget.NewEntry()
	portEntry.SetText(fmt.Sprintf("%d", cfg.Port))

	// 保存文件夹
	saveFolderEntry := widget.NewEntry()
	saveFolderEntry.SetText(cfg.DataDir)
	browseBtn := widget.NewButton(i18n.Translate(lang, i18n.GuiBrowse), func() {
		dlg := dialog.NewFolderOpen(func(uri fyne.ListableURI, err error) {
			if err != nil || uri == nil {
				return
			}
			path := uri.Path()
			saveFolderEntry.SetText(path)
		}, gs.window)
		dlg.Show()
	})

	// 拍照间隔: X 分钟 X 秒
	minutesEntry := widget.NewEntry()
	minutesEntry.SetPlaceHolder(i18n.Translate(lang, i18n.Minutes))
	secondsEntry := widget.NewEntry()
	secondsEntry.SetPlaceHolder(i18n.Translate(lang, i18n.Seconds))

	// 获取当前拍照间隔
	devices, _ := gs.serverApp.DB.ListDevices()
	if len(devices) > 0 {
		interval := devices[0].PhotoInterval
		if interval == 0 {
			interval = 60
		}
		minutesEntry.SetText(fmt.Sprintf("%d", interval/60))
		secondsEntry.SetText(fmt.Sprintf("%d", interval%60))
	} else {
		minutesEntry.SetText("1")
		secondsEntry.SetText("0")
	}

	// 保留天数
	retentionEntry := widget.NewEntry()
	retentionEntry.SetText(fmt.Sprintf("%d", cfg.RetentionDays))

	// 最大存储
	maxStorageEntry := widget.NewEntry()
	maxStorageEntry.SetText(fmt.Sprintf("%d", cfg.MaxStorageMB))

	// 应用到所有设备
	applyToAllCheck := widget.NewCheck(i18n.Translate(lang, i18n.GuiApplyToAll), nil)
	applyToAllCheck.SetChecked(true)

	// 保存按钮
	saveBtn := widget.NewButton(i18n.Translate(lang, i18n.GuiSaveSettings), func() {
		// 解析端口
		port := cfg.Port
		if v := strings.TrimSpace(portEntry.Text); v != "" {
			var p int
			if _, err := fmt.Sscanf(v, "%d", &p); err == nil && p > 0 && p < 65536 {
				port = p
			}
		}

		// 解析拍照间隔
		var minutes, seconds int
		fmt.Sscanf(minutesEntry.Text, "%d", &minutes)
		fmt.Sscanf(secondsEntry.Text, "%d", &seconds)
		totalSeconds := minutes*60 + seconds
		if totalSeconds < 1 {
			totalSeconds = 60
		}

		// 解析保留天数
		var retentionDays int
		fmt.Sscanf(retentionEntry.Text, "%d", &retentionDays)

		// 解析最大存储
		var maxStorage int64
		fmt.Sscanf(maxStorageEntry.Text, "%d", &maxStorage)

		// 保存文件夹
		saveFolder := strings.TrimSpace(saveFolderEntry.Text)
		if saveFolder != "" {
			cleanPath := filepath.Clean(saveFolder)
			if err := os.MkdirAll(cleanPath, 0755); err != nil {
				dialog.ShowError(fmt.Errorf("%v", err), gs.window)
				return
			}
			if err := gs.serverApp.SetDataDir(cleanPath); err != nil {
				dialog.ShowError(fmt.Errorf("%v", err), gs.window)
				return
			}
		}

		// 更新配置
		cfg.Port = port
		cfg.RetentionDays = retentionDays
		cfg.MaxStorageMB = maxStorage
		cfg.DefaultLang = gs.currentLang

		// 保存配置文件
		if err := gs.serverApp.SaveConfig(); err != nil {
			dialog.ShowError(fmt.Errorf("%v", err), gs.window)
			return
		}

		// 更新拍照间隔
		if applyToAllCheck.Checked {
			if err := gs.serverApp.DB.UpdateAllPhotoIntervals(totalSeconds); err != nil {
				slog.Warn("更新拍照间隔失败", "error", err)
			}
		} else if len(devices) > 0 {
			if err := gs.serverApp.DB.UpdatePhotoInterval(devices[0].DeviceID, totalSeconds); err != nil {
				slog.Warn("更新拍照间隔失败", "error", err)
			}
		}

		// 电脑端作为控制中心：把拍照间隔同步到 ESP32 设备。
		ip := strings.TrimSpace(gs.esp32IPEntry.Text)
		secret := strings.TrimSpace(gs.esp32SecretEntry.Text)
		if ip != "" {
			if secret == "" {
				slog.Warn("拍照间隔未下发: 请先在远程控制面板填写设备密钥")
				dialog.ShowError(fmt.Errorf("%s", i18n.Translate(gs.currentLang, i18n.GuiSecretRequired)), gs.window)
			} else if err := gs.serverApp.DevCtrl.SetInterval(ip, 8081, totalSeconds, secret); err != nil {
				slog.Warn("拍照间隔下发失败", "device", ip, "error", err)
				dialog.ShowError(fmt.Errorf("%s: %v", i18n.Translate(gs.currentLang, i18n.GuiIntervalSendFailed), err), gs.window)
			} else {
				slog.Info("拍照间隔已下发", "device", ip, "interval_sec", totalSeconds)
			}
		}

		slog.Info("设置已保存",
			"port", port,
			"save_folder", saveFolder,
			"interval_sec", totalSeconds,
			"retention_days", retentionDays,
			"max_storage_mb", maxStorage,
			"lang", gs.currentLang,
		)

		dialog.ShowInformation(
			i18n.Translate(gs.currentLang, i18n.GuiSaveSuccess),
			i18n.Translate(gs.currentLang, i18n.GuiSaveSuccessMsg),
			window,
		)
	})
	saveBtn.Importance = widget.HighImportance

	// 重启服务器按钮
	restartBtn := widget.NewButton(i18n.Translate(lang, i18n.GuiRestartServer), func() {
		dialog.ShowConfirm(
			i18n.Translate(gs.currentLang, i18n.GuiConfirmRestart),
			i18n.Translate(gs.currentLang, i18n.GuiConfirmRestartMsg),
			func(ok bool) {
				if !ok {
					return
				}
				// 先保存配置
				if err := gs.serverApp.SaveConfig(); err != nil {
					dialog.ShowError(fmt.Errorf("%v", err), gs.window)
					return
				}
				slog.Info("正在重启服务器...")
				gs.statusLabel.SetText(i18n.Translate(gs.currentLang, i18n.GuiStopping))
				gs.updateToggleButton()

				go func() {
					if err := gs.serverApp.ReloadAndRestart(); err != nil {
						slog.Error("服务器重启失败", "error", err)
						dialog.ShowError(fmt.Errorf("%s: %v",
							i18n.Translate(gs.currentLang, i18n.GuiStartFailed), err), gs.window)
						gs.statusLabel.SetText(i18n.Translate(gs.currentLang, i18n.GuiStopped))
						gs.updateToggleButton()
						return
					}
					slog.Info("服务器已重启", "url", gs.serverApp.ServerURL())
					gs.statusLabel.SetText(i18n.Translate(gs.currentLang, i18n.GuiRunning))
					gs.urlLabel.SetText(gs.serverApp.ServerURL())
					gs.updateToggleButton()
					dialog.ShowInformation(
						i18n.Translate(gs.currentLang, i18n.GuiRestartSuccess),
						i18n.Translate(gs.currentLang, i18n.GuiRunning),
						window,
					)
					go gs.pollDeviceStatus()
				}()
			}, gs.window)
	})

	// 修改管理员密码
	newPasswordEntry := widget.NewPasswordEntry()
	newPasswordEntry.SetPlaceHolder(i18n.Translate(lang, i18n.GuiNewPassword))
	changePasswordBtn := widget.NewButton(i18n.Translate(lang, i18n.GuiChangePassword), func() {
		pwd := strings.TrimSpace(newPasswordEntry.Text)
		if pwd == "" {
			dialog.ShowError(fmt.Errorf("%s", i18n.Translate(gs.currentLang, i18n.GuiPasswordEmpty)), gs.window)
			return
		}
		hash, err := config.HashPassword(pwd)
		if err != nil {
			dialog.ShowError(fmt.Errorf("%v", err), gs.window)
			return
		}
		gs.serverApp.Cfg.AdminPasswordHash = hash
		if err := gs.serverApp.SaveConfig(); err != nil {
			dialog.ShowError(fmt.Errorf("%v", err), gs.window)
			return
		}
		newPasswordEntry.SetText("")
		slog.Info("管理员密码已修改")
		dialog.ShowInformation(
			i18n.Translate(gs.currentLang, i18n.GuiPasswordChanged),
			i18n.Translate(gs.currentLang, i18n.GuiPasswordChanged),
			window,
		)
	})

	// 组装设置界面
	settingsContent := container.NewVBox(
		widget.NewCard(i18n.Translate(lang, i18n.GuiSettings), "", container.NewVBox(
			container.NewHBox(widget.NewLabel(i18n.Translate(lang, i18n.GuiListenPort)), portEntry),
			container.NewHBox(widget.NewLabel(i18n.Translate(lang, i18n.GuiDefaultLang)),
				widget.NewLabel(fmt.Sprintf("%s (%s)", i18n.LanguageNames[gs.currentLang], gs.currentLang))),
		)),

		widget.NewCard(i18n.Translate(lang, i18n.GuiPhotoStorage), "", container.NewVBox(
			container.NewHBox(widget.NewLabel(i18n.Translate(lang, i18n.GuiSaveFolder)), saveFolderEntry, browseBtn),
			container.NewHBox(
				widget.NewLabel(i18n.Translate(lang, i18n.GuiPhotoInterval)),
				minutesEntry, widget.NewLabel(i18n.Translate(lang, i18n.Minutes)),
				secondsEntry, widget.NewLabel(i18n.Translate(lang, i18n.Seconds)),
			),
			applyToAllCheck,
		)),

		widget.NewCard(i18n.Translate(lang, i18n.GuiRetentionPolicy), "", container.NewVBox(
			container.NewHBox(widget.NewLabel(i18n.Translate(lang, i18n.GuiRetentionDays)), retentionEntry),
			container.NewHBox(widget.NewLabel(i18n.Translate(lang, i18n.GuiMaxStorage)), maxStorageEntry),
		)),

		widget.NewCard(i18n.Translate(lang, i18n.GuiAdmin), "", container.NewVBox(
			container.NewHBox(widget.NewLabel(i18n.Translate(lang, i18n.GuiNewPassword)), newPasswordEntry, changePasswordBtn),
		)),

		container.NewHBox(saveBtn, restartBtn),
	)

	return container.NewVScroll(settingsContent)
}

// ============================================================
// 日志标签页
// ============================================================

// buildLogsTab 创建日志标签页
func (gs *guiState) buildLogsTab(logEntryPtr **widget.Entry) fyne.CanvasObject {
	lang := gs.currentLang
	logEntry := widget.NewMultiLineEntry()
	logEntry.Wrapping = fyne.TextWrapWord
	logEntry.Disable()
	*logEntryPtr = logEntry

	clearBtn := widget.NewButton(i18n.Translate(lang, i18n.GuiClearLogs), func() {
		gs.logCapture.Clear()
		logEntry.SetText("")
	})

	autoScrollCheck := widget.NewCheck(i18n.Translate(lang, i18n.GuiAutoScroll), nil)
	autoScrollCheck.SetChecked(true)

	content := container.NewBorder(
		container.NewHBox(clearBtn, autoScrollCheck),
		nil, nil, nil,
		container.NewVScroll(logEntry),
	)

	return content
}

// ============================================================
// 多语言实时切换
// ============================================================

// refreshUILanguage 切换语言后刷新所有 UI 文本
func (gs *guiState) refreshUILanguage() {
	lang := gs.currentLang

	// 更新窗口标题
	gs.window.SetTitle(i18n.Translate(lang, i18n.GuiTitle))

	// 更新标签页名称
	if gs.tabs != nil && gs.tabs.Items != nil && len(gs.tabs.Items) >= 3 {
		gs.tabs.Items[0].Text = i18n.Translate(lang, i18n.GuiDashboard)
		gs.tabs.Items[1].Text = i18n.Translate(lang, i18n.GuiSettings)
		gs.tabs.Items[2].Text = i18n.Translate(lang, i18n.GuiLogs)
		gs.tabs.Refresh()
	}

	// 重建仪表盘，让 ESP32 远程控制面板等所有文本同时切换语言。
	if gs.dashContainer != nil {
		gs.dashContainer.Content = gs.buildDashboard()
		gs.dashContainer.Refresh()
	}

	// 重建设置页 (Fyne Card/Label 不支持动态修改标题)。
	gs.settingsContent = gs.buildSettings()

	if gs.tabs != nil && gs.tabs.Items != nil && len(gs.tabs.Items) >= 2 {
		gs.tabs.Items[0].Content = gs.dashContainer
		gs.tabs.Items[1].Content = gs.settingsContent
		gs.tabs.Refresh()
	}

	// 重建后刷新运行状态和按钮文本。
	gs.updateToggleButton()
	if gs.galleryBtn != nil {
		gs.galleryBtn.SetText(i18n.Translate(lang, i18n.GuiOpenGallery))
	}
	if gs.serverApp.Running {
		gs.statusLabel.SetText(i18n.Translate(lang, i18n.GuiRunning))
		gs.urlLabel.SetText(gs.serverApp.ServerURL())
		go gs.updateDashboard()
	} else {
		gs.statusLabel.SetText(i18n.Translate(lang, i18n.GuiStopped))
	}

	// 保存语言到配置。
	gs.serverApp.Cfg.DefaultLang = lang
	_ = gs.serverApp.SaveConfig()
}
