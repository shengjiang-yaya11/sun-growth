// ============================================================
//
//	【电脑端软件】运行目标: Windows / macOS / Linux / NAS 服务器
//	编译工具链: Go 1.21+ (跨平台单二进制, 不依赖 ESP32)
//	运行方式: 直接运行二进制 或 Docker 容器 / systemd 服务
//
// ============================================================
//
// Package main - 生物成长记录仪存储服务器 (商用版)
//
// 单二进制, 跨平台 (Windows/macOS/Linux/NAS)
// 功能:
//   - 接收 ESP32 加密签名上传
//   - 验证签名 + 解密 + 存储照片
//   - 多语言 Web 画廊 (6 种语言)
//   - 管理员登录认证 (bcrypt + SQLite 会话)
//   - MJPEG 延时播放 + MP4 下载
//   - 设备注册管理 (API + Web)
//   - 优雅关闭 / 健康检查 / 结构化日志
//   - 照片保留策略与存储管理
package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"bio-growth-recorder/auth"
	"bio-growth-recorder/config"
	"bio-growth-recorder/database"
	"bio-growth-recorder/i18n"
	"bio-growth-recorder/storage"
	"bio-growth-recorder/util"

	"github.com/gorilla/mux"
)

// 全局实例
var (
	cfg       *config.Config
	sec       *auth.Security
	store     *storage.Storage
	db        *database.DB
	sessMgr   *database.SessionStore
	startTime time.Time
)

// ======================= 中间件 =======================

// statusWriter 包装 http.ResponseWriter 以捕获状态码
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (sw *statusWriter) WriteHeader(code int) {
	sw.status = code
	sw.ResponseWriter.WriteHeader(code)
}

// generateRequestID 生成请求 ID
func generateRequestID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "unknown"
	}
	return hex.EncodeToString(b)
}

// loggingMiddleware 结构化日志中间件 (JSON), 记录请求 ID/方法/路径/状态码/延迟
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		requestID := generateRequestID()
		ww := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(ww, r)
		slog.Info("http 请求",
			"request_id", requestID,
			"method", r.Method,
			"path", r.URL.Path,
			"status", ww.status,
			"duration_ms", time.Since(start).Milliseconds(),
			"remote", r.RemoteAddr,
		)
	})
}

// rateLimitMiddleware 速率限制 (每 IP 每分钟最多 30 个请求)
func rateLimitMiddleware(next http.Handler) http.Handler {
	visitors := make(map[string][]time.Time)
	var mu sync.Mutex
	// 定期清理过期的 IP 记录 (防止内存泄漏)
	lastCleanup := time.Now()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)
		mu.Lock()
		now := time.Now()

		// 每 5 分钟清理一次无活跃请求的 IP
		if now.Sub(lastCleanup) > 5*time.Minute {
			for vIP, timestamps := range visitors {
				var recent []time.Time
				for _, t := range timestamps {
					if now.Sub(t) < time.Minute {
						recent = append(recent, t)
					}
				}
				if len(recent) == 0 {
					delete(visitors, vIP)
				} else {
					visitors[vIP] = recent
				}
			}
			lastCleanup = now
		}

		// 清理 1 分钟前的记录
		var recent []time.Time
		for _, t := range visitors[ip] {
			if now.Sub(t) < time.Minute {
				recent = append(recent, t)
			}
		}
		if len(recent) >= 30 {
			mu.Unlock()
			jsonError(w, "rate limit", http.StatusTooManyRequests)
			return
		}
		recent = append(recent, now)
		visitors[ip] = recent
		mu.Unlock()
		next.ServeHTTP(w, r)
	})
}

// authMiddleware 管理员认证中间件 (Web 路由, 失败重定向到登录页)
func authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("bio_session")
		if err != nil || !sessMgr.Valid(cookie.Value) {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next(w, r)
	}
}

// apiAuthMiddleware 管理员认证中间件 (API 路由, 失败返回 401 JSON)
func apiAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("bio_session")
		if err != nil || !sessMgr.Valid(cookie.Value) {
			jsonError(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// csrfMiddleware CSRF 保护中间件 (double-submit cookie 模式)
// 仅对状态变更方法 (POST/PUT/DELETE) 校验, GET 放行
func csrfMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodDelete:
			if !validateCSRF(r) {
				slog.Warn("CSRF 校验失败", "path", r.URL.Path, "remote", r.RemoteAddr)
				jsonError(w, "csrf validation failed", http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// validateCSRF 校验 double-submit cookie: cookie token 须与表单/请求头 token 一致
func validateCSRF(r *http.Request) bool {
	cookie, err := r.Cookie("bio_csrf")
	if err != nil {
		return false
	}
	token := r.FormValue("csrf_token")
	if token == "" {
		token = r.Header.Get("X-CSRF-Token")
	}
	return auth.ValidateCSRF(cookie.Value, token)
}

// setCSRFCookie 设置 CSRF cookie (非 HttpOnly, 供 JS 读取)
func setCSRFCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     "bio_csrf",
		Value:    token,
		Path:     "/",
		MaxAge:   86400,
		HttpOnly: false, // JS 需读取以发送到请求头
		SameSite: http.SameSiteLaxMode,
		Secure:   cfg.TLSEnabled,
	})
}

// csrfTokenForPage 获取页面所需的 CSRF token (复用现有 cookie 或新建)
func csrfTokenForPage(w http.ResponseWriter, r *http.Request) string {
	if c, err := r.Cookie("bio_csrf"); err == nil && c.Value != "" {
		return c.Value
	}
	token, err := auth.GenerateCSRFToken()
	if err != nil {
		slog.Error("生成 CSRF token 失败", "error", err)
		return ""
	}
	setCSRFCookie(w, token)
	return token
}

// clientIP 提取客户端 IP
func clientIP(r *http.Request) string {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

// 语言检测
func getLang(r *http.Request) string {
	// 1. URL 参数 ?lang=xx
	if lang := r.URL.Query().Get("lang"); lang != "" {
		return lang
	}
	// 2. Cookie
	if cookie, err := r.Cookie("bio_lang"); err == nil {
		return cookie.Value
	}
	// 3. Accept-Language 头
	return i18n.ParseLang(r.Header.Get("Accept-Language"), cfg.DefaultLang)
}

// ======================= API: 上传 =======================

func handleUpload(w http.ResponseWriter, r *http.Request) {
	// 限制请求体大小
	r.Body = http.MaxBytesReader(w, r.Body, cfg.MaxUploadSize)

	encryptedData, err := io.ReadAll(r.Body)
	if err != nil {
		slog.Error("上传: 读取请求体失败", "error", err, "remote", r.RemoteAddr)
		jsonError(w, "read error", http.StatusBadRequest)
		return
	}

	// 从请求头获取认证信息
	deviceID := r.Header.Get("X-Device-ID")
	timestamp := r.Header.Get("X-Timestamp")
	nonce := r.Header.Get("X-Nonce")
	signature := r.Header.Get("X-Signature")
	payloadHash := r.Header.Get("X-Payload-Hash")

	if deviceID == "" || timestamp == "" || nonce == "" || signature == "" || payloadHash == "" {
		slog.Warn("上传: 缺少认证头", "device", deviceID, "remote", r.RemoteAddr)
		jsonError(w, "missing auth headers", http.StatusBadRequest)
		return
	}

	// 读取固件版本 (可选头)
	firmwareVersion := r.Header.Get("X-Firmware-Version")

	// 1. 验证签名
	secret, err := sec.VerifyRequest(deviceID, timestamp, nonce, signature, payloadHash)
	if err != nil {
		slog.Warn("上传: 签名验证失败", "device", deviceID, "error", err)
		jsonError(w, "authentication failed", http.StatusUnauthorized)
		return
	}

	// 2. 解密
	plaintext, err := sec.Decrypt(encryptedData, secret)
	if err != nil {
		slog.Warn("上传: 解密失败", "device", deviceID, "error", err)
		jsonError(w, "decryption failed", http.StatusBadRequest)
		return
	}

	// 3. 验证载荷哈希
	if !sec.VerifyPayloadHash(plaintext, payloadHash) {
		slog.Warn("上传: 哈希不匹配", "device", deviceID)
		jsonError(w, "hash mismatch", http.StatusBadRequest)
		return
	}

	// 4. 验证 JPEG 头
	if len(plaintext) < 2 || plaintext[0] != 0xFF || plaintext[1] != 0xD8 {
		slog.Warn("上传: 非 JPEG 数据", "device", deviceID)
		jsonError(w, "not a valid JPEG", http.StatusBadRequest)
		return
	}

	// 5. 存储文件
	relPath, err := store.SavePhoto(deviceID, plaintext)
	if err != nil {
		slog.Error("上传: 存储失败", "device", deviceID, "error", err)
		jsonError(w, "storage error", http.StatusInternalServerError)
		return
	}

	// 6. 提取 JPEG 尺寸并计算 SHA-256 校验和
	width, height := 0, 0
	if w, h, err := util.JPEGDimensions(plaintext); err == nil {
		width, height = w, h
	} else {
		slog.Warn("上传: JPEG 尺寸解析失败", "device", deviceID, "error", err)
	}
	hash := sha256.Sum256(plaintext)
	checksum := hex.EncodeToString(hash[:]) // SHA-256 校验和

	// 7. 写入数据库照片元数据
	now := time.Now()
	photo := &database.Photo{
		DeviceID:   deviceID,
		FilePath:   relPath,
		FileSize:   int64(len(plaintext)),
		Width:      width,
		Height:     height,
		CapturedAt: now,
		Checksum:   checksum,
	}
	if _, err := db.CreatePhoto(photo); err != nil {
		slog.Error("上传: 写入照片元数据失败", "device", deviceID, "error", err)
		// 文件已存储, 元数据失败不阻断响应
	}

	// 8. 更新设备最后活跃时间
	if err := db.UpdateLastSeen(deviceID); err != nil {
		slog.Warn("上传: 更新设备活跃时间失败", "device", deviceID, "error", err)
	}

	// 9. 更新设备固件版本 (仅在版本变化时写入)
	if firmwareVersion != "" {
		if err := db.UpdateFirmwareVersion(deviceID, firmwareVersion); err != nil {
			slog.Warn("上传: 更新固件版本失败", "device", deviceID, "error", err)
		}
	}

	sizeKB := len(plaintext) / 1024
	slog.Info("上传成功", "device", deviceID, "size_kb", sizeKB, "file", relPath)

	jsonOK(w, map[string]interface{}{
		"status":    "ok",
		"filename":  filepath.Base(relPath),
		"size":      len(plaintext),
		"timestamp": now.Format(time.RFC3339),
	})
}

// ======================= Web: 画廊 =======================

func handleGallery(w http.ResponseWriter, r *http.Request) {
	lang := getLang(r)

	// 设置语言 cookie
	http.SetCookie(w, &http.Cookie{
		Name:   "bio_lang",
		Value:  lang,
		Path:   "/",
		MaxAge: 86400 * 30,
	})

	devices, _ := store.ScanAll()
	totalPhotos, totalSize, deviceCount := store.GetStats()

	// 保留策略信息
	retentionInfo := ""
	if cfg.RetentionDays > 0 {
		retentionInfo = fmt.Sprintf("%d days", cfg.RetentionDays)
	} else {
		retentionInfo = "unlimited"
	}

	data := galleryData{
		Lang:          lang,
		LangName:      i18n.LanguageNames[lang],
		Languages:     i18n.SupportedLanguages,
		LangNames:     i18n.LanguageNames,
		Devices:       devices,
		TotalPhotos:   totalPhotos,
		TotalSize:     totalSize / (1024 * 1024),
		DeviceCount:   deviceCount,
		ServerTime:    time.Now().Format("15:04:05"),
		CSRFToken:     csrfTokenForPage(w, r),
		RetentionDays: cfg.RetentionDays,
		MaxStorageMB:  cfg.MaxStorageMB,
		RetentionInfo: retentionInfo,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	renderGallery(w, data)
}

// ======================= Web: 登录 =======================

func handleLoginPage(w http.ResponseWriter, r *http.Request) {
	lang := getLang(r)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	renderLogin(w, lang, csrfTokenForPage(w, r), false)
}

func handleLoginPost(w http.ResponseWriter, r *http.Request) {
	// 校验 CSRF
	if !validateCSRF(r) {
		slog.Warn("登录: CSRF 校验失败", "remote", r.RemoteAddr)
		jsonError(w, "csrf validation failed", http.StatusForbidden)
		return
	}

	if err := r.ParseForm(); err != nil {
		jsonError(w, "parse form error", http.StatusBadRequest)
		return
	}
	password := r.FormValue("password")

	if cfg.VerifyPassword(password) {
		token, err := sessMgr.Create(clientIP(r))
		if err != nil {
			slog.Error("登录: 创建会话失败", "error", err)
			jsonError(w, "internal error", http.StatusInternalServerError)
			return
		}
		http.SetCookie(w, &http.Cookie{
			Name:     "bio_session",
			Value:    token,
			Path:     "/",
			HttpOnly: true,
			MaxAge:   86400,
			SameSite: http.SameSiteStrictMode,
			Secure:   cfg.TLSEnabled,
		})
		slog.Info("登录成功", "remote", r.RemoteAddr)
		http.Redirect(w, r, "/", http.StatusSeeOther)
	} else {
		slog.Warn("登录失败: 密码错误", "remote", r.RemoteAddr)
		lang := getLang(r)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		renderLogin(w, lang, csrfTokenForPage(w, r), true)
	}
}

func handleLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie("bio_session"); err == nil {
		if err := sessMgr.Destroy(cookie.Value); err != nil {
			slog.Warn("登出: 销毁会话失败", "error", err)
		}
	}
	http.SetCookie(w, &http.Cookie{
		Name:   "bio_session",
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// ======================= API: 照片服务 =======================

func handlePhoto(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	deviceID := vars["deviceID"]
	dateStr := vars["date"]
	filename := vars["filename"]

	// 路径遍历防护: 校验 deviceID / date / filename
	if err := util.ValidatePhotoRequest(deviceID, dateStr, filename); err != nil {
		slog.Warn("照片请求: 参数校验失败", "error", err, "remote", r.RemoteAddr)
		http.NotFound(w, r)
		return
	}

	reader, err := store.GetReader(deviceID, dateStr, filename)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer reader.Close()
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	if _, err := io.Copy(w, reader); err != nil {
		slog.Warn("照片: 写入响应失败", "error", err)
	}
}

// ======================= API: 延时视频 =======================

// scanDatePhotos 扫描某设备某日期目录下的照片 (含路径遍历防护)
func scanDatePhotos(deviceID, dateStr string) ([]string, error) {
	if err := util.ValidatePhotoRequest(deviceID, dateStr, ""); err != nil {
		return nil, err
	}
	path, err := store.GetPhotoPath(deviceID, dateStr, "")
	if err != nil {
		return nil, err
	}
	dir := filepath.Dir(path)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var photos []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".jpg") {
			photos = append(photos, filepath.Join(dir, e.Name()))
		}
	}
	return photos, nil
}

func handleTimelapse(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	deviceID := vars["deviceID"]
	dateStr := vars["date"]

	photos, err := scanDatePhotos(deviceID, dateStr)
	if err != nil || len(photos) == 0 {
		http.NotFound(w, r)
		return
	}

	// MJPEG 流式播放
	w.Header().Set("Content-Type", "multipart/x-mixed-replace; boundary=frameboundary")
	flusher, _ := w.(http.Flusher)

	for _, photo := range photos {
		data, err := os.ReadFile(photo)
		if err != nil {
			continue
		}
		fmt.Fprintf(w, "--frameboundary\r\nContent-Type: image/jpeg\r\nContent-Length: %d\r\n\r\n", len(data))
		w.Write(data)
		w.Write([]byte("\r\n"))
		if flusher != nil {
			flusher.Flush()
		}
		time.Sleep(time.Second / 24) // 24fps
	}
	fmt.Fprint(w, "--frameboundary--\r\n")
}

func handleTimelapseMP4(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	deviceID := vars["deviceID"]
	dateStr := vars["date"]

	// 检查 ffmpeg
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		jsonError(w, "ffmpeg not installed", http.StatusInternalServerError)
		return
	}

	photos, err := scanDatePhotos(deviceID, dateStr)
	if err != nil || len(photos) == 0 {
		http.NotFound(w, r)
		return
	}

	// 创建临时目录, 序号重命名
	tmpDir, err := os.MkdirTemp("", "timelapse-*")
	if err != nil {
		jsonError(w, "temp dir error", http.StatusInternalServerError)
		return
	}
	defer os.RemoveAll(tmpDir)

	for i, p := range photos {
		if err := os.Link(p, filepath.Join(tmpDir, fmt.Sprintf("frame_%05d.jpg", i))); err != nil {
			slog.Warn("延时: 创建硬链接失败", "error", err)
		}
	}

	outputFile := filepath.Join(tmpDir, "timelapse.mp4")
	cmd := exec.Command(ffmpeg, "-y", "-framerate", "24",
		"-i", filepath.Join(tmpDir, "frame_%05d.jpg"),
		"-c:v", "libx264", "-pix_fmt", "yuv420p", "-crf", "23",
		outputFile)
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		jsonError(w, "ffmpeg failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=timelapse_%s_%s.mp4", deviceID, dateStr))
	http.ServeFile(w, r, outputFile)
}

// ======================= API: 统计 =======================

func handleStats(w http.ResponseWriter, r *http.Request) {
	totalPhotos, totalSize, deviceCount := store.GetStats()

	// 数据库统计 (增强)
	dbPhotos, _ := db.CountPhotos()
	dbDevices, _ := db.CountDevices()
	dbSize, _ := db.SumPhotoSize()

	jsonOK(w, map[string]interface{}{
		"total_photos":   totalPhotos,
		"total_size_mb":  totalSize / (1024 * 1024),
		"device_count":   deviceCount,
		"server_time":    time.Now().Format(time.RFC3339),
		"db_photos":      dbPhotos,
		"db_devices":     dbDevices,
		"db_size_mb":     dbSize / (1024 * 1024),
		"retention_days": cfg.RetentionDays,
		"max_storage_mb": cfg.MaxStorageMB,
		"uptime":         int64(time.Since(startTime).Seconds()),
	})
}

// ======================= API: 健康检查 =======================

func handleHealth(w http.ResponseWriter, r *http.Request) {
	deviceCount, _ := db.CountDevices()
	photoCount, _ := db.CountPhotos()
	jsonOK(w, map[string]interface{}{
		"status":  "ok",
		"uptime":  int64(time.Since(startTime).Seconds()),
		"devices": deviceCount,
		"photos":  photoCount,
	})
}

// ======================= API: 拍照模式设置 =======================

// photoModeSettingsReq 拍照模式设置请求
type photoModeSettingsReq struct {
	Minutes    int    `json:"minutes"`
	Seconds    int    `json:"seconds"`
	ApplyToAll bool   `json:"apply_to_all"`
	SaveFolder string `json:"save_folder"`
}

// handleGetPhotoModeSettings 获取拍照模式设置 (GET /api/v1/photo-mode)
func handleGetPhotoModeSettings(w http.ResponseWriter, r *http.Request) {
	devices, err := db.ListDevices()
	if err != nil {
		slog.Error("拍照设置: 查询设备失败", "error", err)
		jsonError(w, "query failed", http.StatusInternalServerError)
		return
	}

	type deviceInterval struct {
		DeviceID      string `json:"device_id"`
		DeviceName    string `json:"device_name"`
		PhotoInterval int    `json:"photo_interval"`
		Minutes       int    `json:"minutes"`
		Seconds       int    `json:"seconds"`
	}

	var list []deviceInterval
	for _, dev := range devices {
		interval := dev.PhotoInterval
		if interval == 0 {
			interval = 60
		}
		list = append(list, deviceInterval{
			DeviceID:      dev.DeviceID,
			DeviceName:    dev.DeviceName,
			PhotoInterval: interval,
			Minutes:       interval / 60,
			Seconds:       interval % 60,
		})
	}

	jsonOK(w, map[string]interface{}{
		"devices":     list,
		"save_folder": cfg.DataDir,
		"custom_dir":  cfg.CustomSaveDir,
		"default_dir": "./captures",
	})
}

// handleUpdatePhotoModeSettings 更新拍照模式设置 (PUT /api/v1/photo-mode)
func handleUpdatePhotoModeSettings(w http.ResponseWriter, r *http.Request) {
	var req photoModeSettingsReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid json", http.StatusBadRequest)
		return
	}

	// 计算总秒数
	totalSeconds := req.Minutes*60 + req.Seconds
	if totalSeconds < 1 {
		jsonError(w, "interval must be at least 1 second", http.StatusBadRequest)
		return
	}

	// 更新自定义保存文件夹 (如果提供)
	if req.SaveFolder != "" {
		// 安全检查: 确保路径合法
		cleanPath := filepath.Clean(req.SaveFolder)
		if err := os.MkdirAll(cleanPath, 0755); err != nil {
			slog.Error("拍照设置: 创建保存目录失败", "path", cleanPath, "error", err)
			jsonError(w, "cannot create save folder", http.StatusBadRequest)
			return
		}

		cfg.CustomSaveDir = cleanPath
		cfg.DataDir = cleanPath

		// 重新初始化存储引擎
		newStore, err := storage.New(cleanPath)
		if err != nil {
			slog.Error("拍照设置: 存储引擎初始化失败", "error", err)
			jsonError(w, "storage init failed", http.StatusInternalServerError)
			return
		}
		store = newStore

		// 保存配置
		configPath := "config.json"
		if len(os.Args) > 1 {
			configPath = os.Args[1]
		}
		if err := config.Save(configPath, cfg); err != nil {
			slog.Warn("拍照设置: 保存配置失败", "error", err)
		}

		slog.Info("拍照设置: 已更新保存目录", "dir", cleanPath)
	}

	// 更新拍照间隔
	if req.ApplyToAll {
		if err := db.UpdateAllPhotoIntervals(totalSeconds); err != nil {
			slog.Error("拍照设置: 更新所有设备间隔失败", "error", err)
			jsonError(w, "update failed", http.StatusInternalServerError)
			return
		}
		slog.Info("拍照设置: 已更新所有设备拍照间隔", "interval_sec", totalSeconds)
	} else {
		// 更新单个设备 (通过 device_id 查询参数)
		deviceID := r.URL.Query().Get("device_id")
		if deviceID == "" {
			// 没有指定设备, 默认更新所有
			if err := db.UpdateAllPhotoIntervals(totalSeconds); err != nil {
				slog.Error("拍照设置: 更新设备间隔失败", "error", err)
				jsonError(w, "update failed", http.StatusInternalServerError)
				return
			}
		} else {
			if err := db.UpdatePhotoInterval(deviceID, totalSeconds); err != nil {
				slog.Error("拍照设置: 更新设备间隔失败", "device", deviceID, "error", err)
				jsonError(w, "update failed", http.StatusInternalServerError)
				return
			}
			slog.Info("拍照设置: 已更新设备拍照间隔", "device", deviceID, "interval_sec", totalSeconds)
		}
	}

	jsonOK(w, map[string]interface{}{
		"status":      "ok",
		"interval":    totalSeconds,
		"minutes":     req.Minutes,
		"seconds":     req.Seconds,
		"save_folder": cfg.DataDir,
	})
}

// ======================= Web: 拍照模式设置页面 =======================

func handlePhotoModePage(w http.ResponseWriter, r *http.Request) {
	lang := getLang(r)
	devices, _ := db.ListDevices()

	var list []photoModeDeviceView
	for _, dev := range devices {
		interval := dev.PhotoInterval
		if interval == 0 {
			interval = 60
		}
		list = append(list, photoModeDeviceView{
			DeviceID:      dev.DeviceID,
			DeviceName:    dev.DeviceName,
			PhotoInterval: interval,
			Minutes:       interval / 60,
			Seconds:       interval % 60,
		})
	}

	data := photoModePageData{
		Lang:       lang,
		LangName:   i18n.LanguageNames[lang],
		Languages:  i18n.SupportedLanguages,
		LangNames:  i18n.LanguageNames,
		Devices:    list,
		SaveFolder: cfg.DataDir,
		CustomDir:  cfg.CustomSaveDir,
		CSRFToken:  csrfTokenForPage(w, r),
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	renderPhotoModePage(w, data)
}

// ======================= API: 删除 =======================

func handleDeleteDay(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	deviceID := vars["deviceID"]
	dateStr := vars["date"]

	// 路径遍历防护
	if err := util.ValidatePhotoRequest(deviceID, dateStr, ""); err != nil {
		slog.Warn("删除: 参数校验失败", "error", err, "remote", r.RemoteAddr)
		jsonError(w, "invalid parameters", http.StatusBadRequest)
		return
	}

	count, err := store.DeleteDay(deviceID, dateStr)
	if err != nil {
		slog.Error("删除: 删除文件失败", "device", deviceID, "date", dateStr, "error", err)
		jsonError(w, "delete failed", http.StatusInternalServerError)
		return
	}

	// 同步删除数据库照片记录
	if _, err := db.DeletePhotosByDeviceDate(deviceID, dateStr); err != nil {
		slog.Warn("删除: 清理数据库记录失败", "device", deviceID, "date", dateStr, "error", err)
	}

	slog.Info("删除日期照片", "device", deviceID, "date", dateStr, "count", count)
	jsonOK(w, map[string]interface{}{"deleted": count})
}

// ======================= API: 设备管理 =======================

// deviceResponse 设备信息响应
type deviceResponse struct {
	DeviceID        string `json:"device_id"`
	DeviceName      string `json:"device_name"`
	SecretHex       string `json:"secret_hex"`
	Status          string `json:"status"`
	FirmwareVersion string `json:"firmware_version"`
	LastSeen        string `json:"last_seen"`
	CreatedAt       string `json:"created_at"`
	PhotoInterval   int    `json:"photo_interval"`
	StorageQuotaMB  int    `json:"storage_quota_mb"`
	PhotoCount      int64  `json:"photo_count"`
}

// registerDeviceReq 注册设备请求
type registerDeviceReq struct {
	DeviceID   string `json:"device_id"`
	DeviceName string `json:"device_name"`
	SecretHex  string `json:"secret_hex"`
}

// handleListDevices 列出所有设备 (GET /api/v1/devices)
func handleListDevices(w http.ResponseWriter, r *http.Request) {
	devices, err := db.ListDevices()
	if err != nil {
		slog.Error("设备列表: 查询失败", "error", err)
		jsonError(w, "query failed", http.StatusInternalServerError)
		return
	}

	var list []deviceResponse
	for _, dev := range devices {
		count, _ := db.CountPhotosByDevice(dev.DeviceID)
		list = append(list, toDeviceResponse(dev, count))
	}
	jsonOK(w, map[string]interface{}{
		"devices": list,
		"total":   len(list),
	})
}

// handleRegisterDevice 注册新设备 (POST /api/v1/devices)
func handleRegisterDevice(w http.ResponseWriter, r *http.Request) {
	var req registerDeviceReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid json", http.StatusBadRequest)
		return
	}

	// 校验设备 ID
	if err := util.ValidateDeviceID(req.DeviceID); err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	// 若未提供密钥, 生成随机 32 字节密钥
	if req.SecretHex == "" {
		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			jsonError(w, "generate secret failed", http.StatusInternalServerError)
			return
		}
		req.SecretHex = hex.EncodeToString(b)
	} else {
		// 校验密钥格式 (64 位十六进制 = 32 字节)
		decoded, err := hex.DecodeString(req.SecretHex)
		if err != nil || len(decoded) != 32 {
			jsonError(w, "invalid secret_hex (must be 64 hex chars)", http.StatusBadRequest)
			return
		}
	}

	// 写入数据库
	if err := db.CreateDevice(req.DeviceID, req.SecretHex, req.DeviceName); err != nil {
		slog.Error("注册设备: 写入数据库失败", "error", err)
		jsonError(w, "device already exists or db error", http.StatusBadRequest)
		return
	}

	// 注册到安全模块 (内存, 用于签名验证)
	if err := sec.RegisterDevice(req.DeviceID, req.SecretHex); err != nil {
		slog.Error("注册设备: 注册安全模块失败", "error", err)
		// 回滚数据库记录
		_ = db.DeleteDevice(req.DeviceID)
		jsonError(w, "register failed", http.StatusInternalServerError)
		return
	}

	dev, _ := db.GetDevice(req.DeviceID)
	slog.Info("注册设备成功", "device", req.DeviceID)
	jsonOK(w, toDeviceResponse(*dev, 0))
}

// handleDeleteDeviceAPI 删除设备 (DELETE /api/v1/devices/{deviceID})
func handleDeleteDeviceAPI(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	deviceID := vars["deviceID"]

	if err := util.ValidateDeviceID(deviceID); err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	// 删除数据库照片记录
	deleted, err := db.DeletePhotosByDevice(deviceID)
	if err != nil {
		slog.Error("删除设备: 清理照片记录失败", "error", err)
		jsonError(w, "delete photos failed", http.StatusInternalServerError)
		return
	}

	// 删除设备记录
	if err := db.DeleteDevice(deviceID); err != nil {
		slog.Error("删除设备: 删除记录失败", "error", err)
		jsonError(w, "delete failed", http.StatusInternalServerError)
		return
	}

	// 从安全模块移除
	sec.RemoveDevice(deviceID)

	// 删除设备文件目录
	if err := store.DeleteDevice(deviceID); err != nil {
		slog.Warn("删除设备: 清理文件目录失败", "device", deviceID, "error", err)
	}

	slog.Info("删除设备成功", "device", deviceID, "photos_deleted", deleted)
	jsonOK(w, map[string]interface{}{"deleted": true, "device_id": deviceID})
}

// handleListDevicePhotos 列出设备照片 (分页) (GET /api/v1/devices/{deviceID}/photos)
func handleListDevicePhotos(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	deviceID := vars["deviceID"]

	if err := util.ValidateDeviceID(deviceID); err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	// 分页参数
	limit := 50
	offset := 0
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}

	photos, err := db.ListPhotosByDevice(deviceID, limit, offset)
	if err != nil {
		slog.Error("设备照片: 查询失败", "error", err)
		jsonError(w, "query failed", http.StatusInternalServerError)
		return
	}
	total, _ := db.CountPhotosByDevice(deviceID)

	type photoItem struct {
		PhotoID    int64  `json:"photo_id"`
		DeviceID   string `json:"device_id"`
		FilePath   string `json:"file_path"`
		FileSize   int64  `json:"file_size"`
		CapturedAt string `json:"captured_at"`
		URL        string `json:"url"`
	}

	var items []photoItem
	for _, p := range photos {
		url := fmt.Sprintf("/photo/%s/%s/%s", p.DeviceID,
			p.CapturedAt.Format("2006-01-02"), filepath.Base(p.FilePath))
		items = append(items, photoItem{
			PhotoID:    p.PhotoID,
			DeviceID:   p.DeviceID,
			FilePath:   p.FilePath,
			FileSize:   p.FileSize,
			CapturedAt: p.CapturedAt.Format(time.RFC3339),
			URL:        url,
		})
	}

	jsonOK(w, map[string]interface{}{
		"photos": items,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// ======================= Web: 设备管理页面 =======================

func handleDevicesPage(w http.ResponseWriter, r *http.Request) {
	lang := getLang(r)
	devices, _ := db.ListDevices()

	var list []deviceView
	for _, dev := range devices {
		count, _ := db.CountPhotosByDevice(dev.DeviceID)
		lastSeen := ""
		if !dev.LastSeen.IsZero() {
			lastSeen = dev.LastSeen.Format("2006-01-02 15:04:05")
		}
		list = append(list, deviceView{
			DeviceID:   dev.DeviceID,
			DeviceName: dev.DeviceName,
			SecretHex:  dev.SecretHex,
			Status:     dev.Status,
			LastSeen:   lastSeen,
			PhotoCount: count,
		})
	}

	data := devicesPageData{
		Lang:      lang,
		LangName:  i18n.LanguageNames[lang],
		Languages: i18n.SupportedLanguages,
		LangNames: i18n.LanguageNames,
		Devices:   list,
		CSRFToken: csrfTokenForPage(w, r),
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	renderDevicesPage(w, data)
}

// ======================= 辅助函数 =======================

func toDeviceResponse(dev database.Device, photoCount int64) deviceResponse {
	lastSeen := ""
	if !dev.LastSeen.IsZero() {
		lastSeen = dev.LastSeen.Format(time.RFC3339)
	}
	createdAt := ""
	if !dev.CreatedAt.IsZero() {
		createdAt = dev.CreatedAt.Format(time.RFC3339)
	}
	return deviceResponse{
		DeviceID:        dev.DeviceID,
		DeviceName:      dev.DeviceName,
		SecretHex:       dev.SecretHex,
		Status:          dev.Status,
		FirmwareVersion: dev.FirmwareVersion,
		LastSeen:        lastSeen,
		CreatedAt:       createdAt,
		PhotoInterval:   dev.PhotoInterval,
		StorageQuotaMB:  dev.StorageQuotaMB,
		PhotoCount:      photoCount,
	}
}

func jsonOK(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		slog.Error("JSON 编码失败", "error", err)
	}
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(map[string]string{"error": msg}); err != nil {
		slog.Error("JSON 编码失败", "error", err)
	}
}

// ======================= 主函数 =======================

func main() {
	// 初始化结构化日志 (JSON 格式)
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	// 解析命令行参数
	configPath := "config.json"
	headless := false

	for _, arg := range os.Args[1:] {
		switch arg {
		case "--headless", "-h":
			headless = true
		case "--help":
			fmt.Println("生物成长记录仪 - 存储服务器 (商用版)")
			fmt.Println("作者: Andrew 亚生")
			fmt.Println()
			fmt.Println("用法:")
			fmt.Println("  bio-recorder-server [config.json]           GUI 窗口模式")
			fmt.Println("  bio-recorder-server --headless [config.json] 命令行模式")
			fmt.Println()
			fmt.Println("参数:")
			fmt.Println("  --headless, -h   以命令行模式运行 (无 GUI 窗口)")
			fmt.Println("  --help           显示帮助信息")
			fmt.Println("  config.json      配置文件路径 (默认: config.json)")
			os.Exit(0)
		default:
			if !strings.HasPrefix(arg, "-") {
				configPath = arg
			}
		}
	}

	// 创建服务器实例
	serverApp, err := NewServerApp(configPath)
	if err != nil {
		slog.Error("服务器初始化失败", "error", err)
		os.Exit(1)
	}

	// 根据模式启动
	if headless {
		runHeadless(serverApp)
	} else {
		runGUI(serverApp)
	}
}
