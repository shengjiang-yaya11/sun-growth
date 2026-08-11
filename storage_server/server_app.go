// ============================================================
//  【电脑端软件】运行目标: Windows / macOS / Linux / NAS 服务器
//  编译工具链: Go 1.21+ (跨平台单二进制, 不依赖 ESP32)
//  运行方式: GUI 窗口模式 或 --headless 命令行模式
// ============================================================
//
// server_app.go - 服务器生命周期管理
//
// 将服务器初始化/启动/关闭逻辑从 main() 中提取,
// 使 GUI 模式和 headless 模式均可复用

package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"bio-growth-recorder/auth"
	"bio-growth-recorder/config"
	"bio-growth-recorder/database"
	"bio-growth-recorder/i18n"
	"bio-growth-recorder/storage"

	"github.com/gorilla/mux"
)

// ServerApp 封装服务器全部状态和生命周期
type ServerApp struct {
	Cfg       *config.Config
	DevCtrl   *DeviceController  // v3.1: ESP32 设备双向控制器
	DB        *database.DB
	Sec       *auth.Security
	Store     *storage.Storage
	SessMgr   *database.SessionStore
	Server    *http.Server
	Router    *mux.Router
	Cancel    context.CancelFunc
	StartTime time.Time
	Running   bool // 服务器是否正在运行

	configPath string
	localIP    string
	mu         sync.Mutex
}

// NewServerApp 创建服务器实例 (不启动)
func NewServerApp(configPath string) (*ServerApp, error) {
	app := &ServerApp{
		configPath: configPath,
		StartTime:  time.Now(),
	}

	// 1. 加载配置
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, fmt.Errorf("配置加载失败: %w", err)
	}
	app.Cfg = cfg

	// 2. 初始化数据库
	db, err := database.Open(cfg.DBPath)
	if err != nil {
		return nil, fmt.Errorf("数据库初始化失败: %w", err)
	}
	app.DB = db

	// 3. 初始化安全模块
	app.Sec = auth.New(map[string]string{}, cfg.TimestampTolerance)

	// 4. 迁移旧配置设备并加载数据库设备
	app.migrateConfigDevices()
	app.loadDevicesFromDB()

	// 5. 初始化存储
	store, err := storage.New(cfg.DataDir)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("存储初始化失败: %w", err)
	}
	app.Store = store

	// 6. 初始化会话管理
	app.SessMgr = database.NewSessionStore(db, cfg.TLSEnabled)

	// 7. 获取局域网 IP
	app.localIP = app.getLocalIP()

	// 8. 创建设备控制器 (v3.1)
	app.DevCtrl = NewDeviceController()

	// 9. 构建路由
	app.buildRouter()

	return app, nil
}

// Start 启动 HTTP 服务器 (非阻塞)
func (app *ServerApp) Start() error {
	app.mu.Lock()
	defer app.mu.Unlock()

	if app.Running {
		return fmt.Errorf("服务器已在运行")
	}

	// 设置全局变量 (供 HTTP handler 使用)
	cfg = app.Cfg
	db = app.DB
	sec = app.Sec
	store = app.Store
	sessMgr = app.SessMgr
	app.StartTime = time.Now()
	startTime = app.StartTime

	// 初始化保留策略
	ctx, cancel := context.WithCancel(context.Background())
	app.Cancel = cancel
	retention := storage.NewRetentionManager(app.DB, app.Cfg.DataDir, app.Cfg.RetentionDays, app.Cfg.MaxStorageMB)
	retention.Start(ctx)

	addr := fmt.Sprintf("%s:%d", app.Cfg.Host, app.Cfg.Port)
	slog.Info("服务器启动", "addr", addr, "tls", app.Cfg.TLSEnabled)

	app.Server = &http.Server{
		Addr:         addr,
		Handler:      app.Router,
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 300 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		var err error
		if app.Cfg.TLSEnabled {
			err = app.Server.ListenAndServeTLS(app.Cfg.TLSCert, app.Cfg.TLSKey)
		} else {
			err = app.Server.ListenAndServe()
		}
		if err != nil && err != http.ErrServerClosed {
			slog.Error("服务器异常退出", "error", err)
		}
	}()

	app.Running = true
	return nil
}

// Stop 停止 HTTP 服务器 (不关闭数据库, 允许重启)
func (app *ServerApp) Stop() {
	app.mu.Lock()
	defer app.mu.Unlock()

	if !app.Running {
		return
	}

	if app.Cancel != nil {
		app.Cancel()
		app.Cancel = nil
	}
	if app.Server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := app.Server.Shutdown(ctx); err != nil {
			slog.Error("优雅关闭失败", "error", err)
		}
		app.Server = nil
	}
	app.Running = false
	slog.Info("服务器已停止")
}

// Shutdown 完全关闭: 停止服务器并关闭数据库 (程序退出时调用)
func (app *ServerApp) Shutdown() {
	app.Stop()
	app.mu.Lock()
	defer app.mu.Unlock()
	if app.DB != nil {
		app.DB.Close()
		app.DB = nil
	}
	slog.Info("服务器已完全关闭")
}

// Restart 重启服务器 (不重新加载配置)
func (app *ServerApp) Restart() error {
	app.Stop()
	// 重新构建路由 (捕获配置变更)
	app.buildRouter()
	return app.Start()
}

// ReloadAndRestart 重新加载配置并重启服务器
func (app *ServerApp) ReloadAndRestart() error {
	app.Stop()

	// 重新加载配置
	newCfg, err := config.Load(app.configPath)
	if err != nil {
		return fmt.Errorf("配置加载失败: %w", err)
	}
	app.Cfg = newCfg

	// 重新初始化存储 (目录可能已变更)
	newStore, err := storage.New(app.Cfg.DataDir)
	if err != nil {
		return fmt.Errorf("存储初始化失败: %w", err)
	}
	app.Store = newStore

	// 重新构建路由
	app.buildRouter()

	return app.Start()
}

// LocalIP 返回局域网 IP
func (app *ServerApp) LocalIP() string {
	return app.localIP
}

// ServerURL 返回服务器访问 URL
func (app *ServerApp) ServerURL() string {
	scheme := "http"
	if app.Cfg.TLSEnabled {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s:%d", scheme, app.localIP, app.Cfg.Port)
}

// getLocalIP 获取本机局域网 IPv4 地址
// 过滤链路本地地址 (APIPA: 169.254.x.x) 和回环地址
func (app *ServerApp) getLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "127.0.0.1"
	}
	var fallback string
	for _, addr := range addrs {
		if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
			ip4 := ipNet.IP.To4()
			if ip4 == nil {
				continue
			}
			// 过滤链路本地地址 (APIPA: 169.254.0.0/16)
			if ip4[0] == 169 && ip4[1] == 254 {
				fallback = ip4.String()
				continue
			}
			return ip4.String()
		}
	}
	// 如果只有 APIPA 地址, 回退到 localhost (APIPA 不可用于 ESP32 连接)
	if fallback != "" {
		return "127.0.0.1"
	}
	return "127.0.0.1"
}

// migrateConfigDevices 将旧 config.json 中的设备迁移到数据库
func (app *ServerApp) migrateConfigDevices() {
	for id, secretHex := range app.Cfg.Devices {
		if _, err := app.DB.GetDevice(id); err == nil {
			_ = app.Sec.RegisterDevice(id, secretHex)
			continue
		}
		if err := app.DB.CreateDevice(id, secretHex, id); err != nil {
			slog.Warn("设备迁移失败", "device", id, "error", err)
			continue
		}
		slog.Info("已迁移配置文件设备到数据库", "device", id)
	}
}

// loadDevicesFromDB 从数据库加载所有设备到安全模块
func (app *ServerApp) loadDevicesFromDB() {
	devices, err := app.DB.ListDevices()
	if err != nil {
		slog.Error("从数据库加载设备失败", "error", err)
		return
	}
	for _, dev := range devices {
		if err := app.Sec.RegisterDevice(dev.DeviceID, dev.SecretHex); err != nil {
			slog.Warn("注册设备到安全模块失败", "device", dev.DeviceID, "error", err)
		}
	}
	slog.Info("从数据库加载设备完成", "count", len(devices))
}

// buildRouter 构建 HTTP 路由
func (app *ServerApp) buildRouter() {
	r := mux.NewRouter()
	r.Use(loggingMiddleware)
	r.Use(rateLimitMiddleware)

	// 健康检查 (无需认证)
	r.HandleFunc("/health", handleHealth).Methods("GET")

	// API: 设备管理 (需要管理员认证 + CSRF)
	apiAuth := r.PathPrefix("/api/v1").Subrouter()
	apiAuth.Use(apiAuthMiddleware)
	apiAuth.Use(csrfMiddleware)
	apiAuth.HandleFunc("/devices", handleListDevices).Methods("GET")
	apiAuth.HandleFunc("/devices", handleRegisterDevice).Methods("POST")
	apiAuth.HandleFunc("/devices/{deviceID}", handleDeleteDeviceAPI).Methods("DELETE")
	apiAuth.HandleFunc("/devices/{deviceID}/photos", handleListDevicePhotos).Methods("GET")
	apiAuth.HandleFunc("/stats", handleStats).Methods("GET")
	apiAuth.HandleFunc("/delete/{deviceID}/{date}", handleDeleteDay).Methods("DELETE")
	apiAuth.HandleFunc("/photo-mode", handleGetPhotoModeSettings).Methods("GET")
	apiAuth.HandleFunc("/photo-mode", handleUpdatePhotoModeSettings).Methods("PUT")

	// API: 上传 (ESP32 调用, 无需登录)
	r.HandleFunc("/api/v1/upload", handleUpload).Methods("POST")

	// Web 路由 (需要登录)
	r.HandleFunc("/login", handleLoginPage).Methods("GET")
	r.HandleFunc("/login", handleLoginPost).Methods("POST")
	r.HandleFunc("/logout", handleLogout).Methods("GET")
	r.HandleFunc("/", authMiddleware(handleGallery)).Methods("GET")
	r.HandleFunc("/devices", authMiddleware(handleDevicesPage)).Methods("GET")
	r.HandleFunc("/photo-mode", authMiddleware(handlePhotoModePage)).Methods("GET")
	r.HandleFunc("/photo/{deviceID}/{date}/{filename}", authMiddleware(handlePhoto)).Methods("GET")
	r.HandleFunc("/timelapse/{deviceID}/{date}", authMiddleware(handleTimelapse)).Methods("GET")
	r.HandleFunc("/timelapse/mp4/{deviceID}/{date}", authMiddleware(handleTimelapseMP4)).Methods("GET")

	// 静态文件
	r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	app.Router = r
}

// PrintBanner 打印启动信息
func (app *ServerApp) PrintBanner() {
	fmt.Println()
	fmt.Println("========================================================")
	fmt.Println("   生物成长记录仪 - 存储服务器 (商用版)")
	fmt.Println("   作者: Andrew 亚生")
	fmt.Println("========================================================")
	fmt.Printf("  平台       : %s/%s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Printf("  数据目录   : %s\n", app.Cfg.DataDir)
	fmt.Printf("  数据库     : %s\n", app.Cfg.DBPath)
	fmt.Printf("  Web 画廊   : %s\n", app.ServerURL())
	fmt.Printf("  上传接口   : %s/api/v1/upload\n", app.ServerURL())
	fmt.Printf("  健康检查   : %s/health\n", app.ServerURL())
	fmt.Printf("  已注册设备 : %d 台\n", len(app.Sec.ListDevices()))
	fmt.Printf("  支持语言   : %s\n", strings.Join(i18n.SupportedLanguages, ", "))
	fmt.Printf("  保留策略   : %d 天 / 上限 %d MB\n", app.Cfg.RetentionDays, app.Cfg.MaxStorageMB)
	fmt.Println()
	fmt.Println("  ESP32 端请配置 SERVER_HOST 和 SERVER_PORT 为:")
	fmt.Printf("    %s\n", app.ServerURL())
	fmt.Println("========================================================")
	fmt.Println()
}

// SaveConfig 保存当前配置到文件
func (app *ServerApp) SaveConfig() error {
	return config.Save(app.configPath, app.Cfg)
}

// OpenBrowser 打开系统默认浏览器
func OpenBrowser(url string) {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "windows":
		cmd = "rundll32"
		args = []string{"url.dll,FileProtocolHandler", url}
	case "darwin":
		cmd = "open"
		args = []string{url}
	default: // linux, freebsd, etc.
		cmd = "xdg-open"
		args = []string{url}
	}

	if _, err := os.Stat("/.dockerenv"); err == nil {
		// Docker 环境下无法打开浏览器
		slog.Info("Docker 环境, 请手动打开浏览器", "url", url)
		return
	}

	execCmd := exec.Command(cmd, args...)
	if err := execCmd.Start(); err != nil {
		slog.Warn("打开浏览器失败", "error", err, "url", url)
	}
}
