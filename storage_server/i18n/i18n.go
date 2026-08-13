// ============================================================
//
//	【电脑端软件】运行目标: Windows / macOS / Linux / NAS 服务器
//	编译工具链: Go 1.21+ (跨平台单二进制, 不依赖 ESP32)
//	运行方式: 直接运行二进制 或 Docker 容器 / systemd 服务
//
// ============================================================
//
// Package i18n - 多语言国际化
// 支持: 英语(en), 简体中文(zh-CN), 繁体中文(zh-TW), 日语(ja), 西班牙语(es), 法语(fr)
package i18n

import (
	"fmt"
	"strings"
)

// Lang 支持的语言
const (
	LangEn   = "en"
	LangZhCN = "zh-CN"
	LangZhTW = "zh-TW"
	LangJa   = "ja"
	LangEs   = "es"
	LangFr   = "fr"
)

// 支持的语言列表
var SupportedLanguages = []string{LangEn, LangZhCN, LangZhTW, LangJa, LangEs, LangFr}

// 语言显示名称
var LanguageNames = map[string]string{
	LangEn:   "English",
	LangZhCN: "简体中文",
	LangZhTW: "繁體中文",
	LangJa:   "日本語",
	LangEs:   "Español",
	LangFr:   "Français",
}

// 翻译键
type Key string

const (
	// 通用
	Title    Key = "title"
	Subtitle Key = "subtitle"
	Loading  Key = "loading"
	Refresh  Key = "refresh"
	Close    Key = "close"
	Delete   Key = "delete"
	Confirm  Key = "confirm"
	Cancel   Key = "cancel"

	// 统计
	TotalPhotos Key = "total_photos"
	DeviceCount Key = "device_count"
	TotalSize   Key = "total_size"
	ServerTime  Key = "server_time"

	// 画廊
	NoPhotos      Key = "no_photos"
	WaitingPhotos Key = "waiting_photos"
	UploadAddress Key = "upload_address"
	Device        Key = "device"
	Photos        Key = "photos"
	Date          Key = "date"

	// 操作
	PlayTimelapse    Key = "play_timelapse"
	DownloadMP4      Key = "download_mp4"
	PrevPhoto        Key = "prev_photo"
	NextPhoto        Key = "next_photo"
	Generating       Key = "generating"
	PlayingTimelapse Key = "playing_timelapse"

	// 登录
	Login         Key = "login"
	Password      Key = "password"
	Logout        Key = "logout"
	LoginRequired Key = "login_required"
	WrongPassword Key = "wrong_password"

	// 设备管理
	Devices        Key = "devices"
	RegisterDevice Key = "register_device"
	DeviceID       Key = "device_id"
	DeviceSecret   Key = "device_secret"
	DeviceName     Key = "device_name"
	Status         Key = "status"
	LastSeen       Key = "last_seen"

	// 存储
	Retention Key = "retention"

	// 拍照模式设置
	PhotoMode         Key = "photo_mode"
	PhotoInterval     Key = "photo_interval"
	Minutes           Key = "minutes"
	Seconds           Key = "seconds"
	SaveFolder        Key = "save_folder"
	Browse            Key = "browse"
	SaveSettings      Key = "save_settings"
	SettingsSaved     Key = "settings_saved"
	PhotoModeSettings Key = "photo_mode_settings"
	CustomFolder      Key = "custom_folder"
	DefaultFolder     Key = "default_folder"
	CurrentInterval   Key = "current_interval"
	IntervalHint      Key = "interval_hint"
	FolderHint        Key = "folder_hint"
	ApplyToAll        Key = "apply_to_all"
	SettingsDesc      Key = "settings_desc"

	// GUI 专用
	GuiTitle             Key = "gui_title"
	GuiDashboard         Key = "gui_dashboard"
	GuiSettings          Key = "gui_settings"
	GuiLogs              Key = "gui_logs"
	GuiStartServer       Key = "gui_start_server"
	GuiStopServer        Key = "gui_stop_server"
	GuiRestartServer     Key = "gui_restart_server"
	GuiOpenGallery       Key = "gui_open_gallery"
	GuiServerStatus      Key = "gui_server_status"
	GuiAccessURL         Key = "gui_access_url"
	GuiUptime            Key = "gui_uptime"
	GuiTotalPhotos       Key = "gui_total_photos"
	GuiStorageUsed       Key = "gui_storage_used"
	GuiDeviceStatus      Key = "gui_device_status"
	GuiRunning           Key = "gui_running"
	GuiStopped           Key = "gui_stopped"
	GuiStarting          Key = "gui_starting"
	GuiStopping          Key = "gui_stopping"
	GuiListenPort        Key = "gui_listen_port"
	GuiDefaultLang       Key = "gui_default_lang"
	GuiPhotoStorage      Key = "gui_photo_storage"
	GuiSaveFolder        Key = "gui_save_folder"
	GuiBrowse            Key = "gui_browse"
	GuiPhotoInterval     Key = "gui_photo_interval"
	GuiApplyToAll        Key = "gui_apply_to_all"
	GuiRetentionPolicy   Key = "gui_retention_policy"
	GuiRetentionDays     Key = "gui_retention_days"
	GuiMaxStorage        Key = "gui_max_storage"
	GuiAdmin             Key = "gui_admin"
	GuiNewPassword       Key = "gui_new_password"
	GuiChangePassword    Key = "gui_change_password"
	GuiSaveSettings      Key = "gui_save_settings"
	GuiClearLogs         Key = "gui_clear_logs"
	GuiAutoScroll        Key = "gui_auto_scroll"
	GuiLanguage          Key = "gui_language"
	GuiConfirmRestart    Key = "gui_confirm_restart"
	GuiConfirmRestartMsg Key = "gui_confirm_restart_msg"
	GuiRestartSuccess    Key = "gui_restart_success"
	GuiSaveSuccess       Key = "gui_save_success"
	GuiSaveSuccessMsg    Key = "gui_save_success_msg"
	GuiStartFailed       Key = "gui_start_failed"
	GuiPasswordChanged   Key = "gui_password_changed"
	GuiPasswordEmpty     Key = "gui_password_empty"
	GuiOnline            Key = "gui_online"
	GuiOffline           Key = "gui_offline"
	GuiNever             Key = "gui_never"
	GuiUnknown           Key = "gui_unknown"
	GuiFirmwareVer       Key = "gui_firmware_ver"
	GuiPhotoCount        Key = "gui_photo_count"
	GuiDeviceName        Key = "gui_device_name"

	// ESP32 设置向导
	GuiESP32Setup         Key = "gui_esp32_setup"
	GuiESP32Step1         Key = "gui_esp32_step1"
	GuiESP32Step2         Key = "gui_esp32_step2"
	GuiESP32Step3         Key = "gui_esp32_step3"
	GuiServerForESP32     Key = "gui_server_for_esp32"
	GuiSoftAPHint         Key = "gui_softap_hint"
	GuiWaitingESP32       Key = "gui_waiting_esp32"
	GuiCheckDevices       Key = "gui_check_devices"
	GuiNoDevicesYet       Key = "gui_no_devices_yet"
	GuiCopyURL            Key = "gui_copy_url"
	GuiURLCopied          Key = "gui_url_copied"
	GuiESP32Control       Key = "gui_esp32_control"
	GuiDeviceIP           Key = "gui_device_ip"
	GuiDeviceSecretLabel  Key = "gui_device_secret_label"
	GuiRemoteCapture      Key = "gui_remote_capture"
	GuiCaptureOnce        Key = "gui_capture_once"
	GuiScheduleCapture    Key = "gui_schedule_capture"
	GuiDelaySeconds       Key = "gui_delay_seconds"
	GuiScanDevices        Key = "gui_scan_devices"
	GuiRefreshStatus      Key = "gui_refresh_status"
	GuiDiscovering        Key = "gui_discovering"
	GuiDevicesFound       Key = "gui_devices_found"
	GuiNoDevicesFound     Key = "gui_no_devices_found"
	GuiCaptureSent        Key = "gui_capture_sent"
	GuiCaptureQueued      Key = "gui_capture_queued"
	GuiCaptureFailed      Key = "gui_capture_failed"
	GuiIPRequired         Key = "gui_ip_required"
	GuiSecretRequired     Key = "gui_secret_required"
	GuiIntervalSendFailed Key = "gui_interval_send_failed"
	GuiNotConnected       Key = "gui_not_connected"
	GuiOfflineUnreachable Key = "gui_offline_unreachable"
	GuiSignal             Key = "gui_signal"
	GuiFreeHeap           Key = "gui_free_heap"
	GuiScheduleInvalid    Key = "gui_schedule_invalid"
	GuiScheduled          Key = "gui_scheduled"

	// 错误
	ErrorUpload       Key = "error_upload"
	ErrorNotFound     Key = "error_not_found"
	ErrorUnauthorized Key = "error_unauthorized"
)

// translations 所有翻译
var translations = map[string]map[Key]string{
	LangEn: {
		Title:                 "Bio Growth Recorder",
		Subtitle:              "Photo Gallery",
		Loading:               "Loading...",
		Refresh:               "Refresh",
		Close:                 "Close",
		Delete:                "Delete",
		Confirm:               "Confirm",
		Cancel:                "Cancel",
		TotalPhotos:           "Photos",
		DeviceCount:           "Devices",
		TotalSize:             "MB Storage",
		ServerTime:            "Server Time",
		NoPhotos:              "No photos yet",
		WaitingPhotos:         "Waiting for ESP32 to upload photos...",
		UploadAddress:         "ESP32 upload URL",
		Device:                "Device",
		Photos:                "photos",
		Date:                  "Date",
		PlayTimelapse:         "Play Timelapse",
		DownloadMP4:           "Download MP4",
		PrevPhoto:             "Previous",
		NextPhoto:             "Next",
		Generating:            "Generating...",
		PlayingTimelapse:      "Playing timelapse... %d photos @ 24fps",
		Login:                 "Login",
		Password:              "Password",
		Logout:                "Logout",
		LoginRequired:         "Login required to view gallery",
		WrongPassword:         "Wrong password",
		Devices:               "Devices",
		RegisterDevice:        "Register Device",
		DeviceID:              "Device ID",
		DeviceSecret:          "Device Secret (hex)",
		DeviceName:            "Device Name",
		Status:                "Status",
		LastSeen:              "Last Seen",
		Retention:             "Retention",
		PhotoMode:             "Photo Mode",
		PhotoInterval:         "Photo Interval",
		Minutes:               "min",
		Seconds:               "sec",
		SaveFolder:            "Save Folder",
		Browse:                "Browse",
		SaveSettings:          "Save Settings",
		SettingsSaved:         "Settings saved successfully",
		PhotoModeSettings:     "Photo Mode Settings",
		CustomFolder:          "Custom Folder",
		DefaultFolder:         "Default Folder",
		CurrentInterval:       "Current Interval",
		IntervalHint:          "Set how often the camera takes a photo (e.g. 1 min 30 sec = every 90 seconds)",
		FolderHint:            "Choose where received photos are saved on this computer",
		ApplyToAll:            "Apply to all devices",
		SettingsDesc:          "Configure photo capture schedule and storage location",
		GuiTitle:              "Bio Growth Recorder - Storage Server",
		GuiDashboard:          "Dashboard",
		GuiSettings:           "Settings",
		GuiLogs:               "Logs",
		GuiStartServer:        "Start Server",
		GuiStopServer:         "Stop Server",
		GuiRestartServer:      "Restart Server",
		GuiOpenGallery:        "Open Web Gallery",
		GuiServerStatus:       "Server Status",
		GuiAccessURL:          "Access URL:",
		GuiUptime:             "Uptime:",
		GuiTotalPhotos:        "Total Photos:",
		GuiStorageUsed:        "Storage Used:",
		GuiDeviceStatus:       "ESP32 Device Status",
		GuiRunning:            "● Running",
		GuiStopped:            "● Stopped",
		GuiStarting:           "● Starting...",
		GuiStopping:           "● Stopping...",
		GuiListenPort:         "Listen Port:",
		GuiDefaultLang:        "Default Language:",
		GuiPhotoStorage:       "Photo Storage",
		GuiSaveFolder:         "Save Folder:",
		GuiBrowse:             "Browse...",
		GuiPhotoInterval:      "Photo Interval:",
		GuiApplyToAll:         "Apply to all devices",
		GuiRetentionPolicy:    "Retention Policy",
		GuiRetentionDays:      "Retention days (0=forever):",
		GuiMaxStorage:         "Max storage MB (0=unlimited):",
		GuiAdmin:              "Administrator",
		GuiNewPassword:        "New Password:",
		GuiChangePassword:     "Change Password",
		GuiSaveSettings:       "Save Settings",
		GuiClearLogs:          "Clear Logs",
		GuiAutoScroll:         "Auto Scroll",
		GuiLanguage:           "Language:",
		GuiConfirmRestart:     "Confirm Restart",
		GuiConfirmRestartMsg:  "Restarting the server will briefly interrupt service. Continue?",
		GuiRestartSuccess:     "Restart Successful",
		GuiSaveSuccess:        "Save Successful",
		GuiSaveSuccessMsg:     "Settings saved. Port and language changes require server restart.",
		GuiStartFailed:        "Server start failed",
		GuiPasswordChanged:    "Password Changed",
		GuiPasswordEmpty:      "Password cannot be empty",
		GuiOnline:             "● Online",
		GuiOffline:            "● Offline",
		GuiNever:              "Never",
		GuiUnknown:            "Unknown",
		GuiFirmwareVer:        "Firmware",
		GuiPhotoCount:         "Photos",
		GuiDeviceName:         "Name",
		GuiESP32Setup:         "ESP32 Setup Guide",
		GuiESP32Step1:         "1. Power on ESP32, find WiFi network: BioRecorder-XXXX",
		GuiESP32Step2:         "2. Connect to it, open 192.168.4.1, enter WiFi & server URL below",
		GuiESP32Step3:         "3. ESP32 will auto-connect and start uploading photos",
		GuiServerForESP32:     "Server URL for ESP32:",
		GuiSoftAPHint:         "SSID: BioRecorder-XXXX (XXXX = last 4 of MAC)",
		GuiWaitingESP32:       "Waiting for ESP32 to connect...",
		GuiCheckDevices:       "Check Connected Devices",
		GuiNoDevicesYet:       "No ESP32 connected yet. Follow the steps above.",
		GuiCopyURL:            "Copy URL",
		GuiURLCopied:          "URL copied to clipboard",
		GuiESP32Control:       "ESP32 Remote Control",
		GuiDeviceIP:           "Device IP:",
		GuiDeviceSecretLabel:  "Device Secret:",
		GuiRemoteCapture:      "📷 Capture Now",
		GuiCaptureOnce:        "⏱️ Schedule Capture",
		GuiScheduleCapture:    "Schedule one capture",
		GuiDelaySeconds:       "Delay (seconds):",
		GuiScanDevices:        "🔍 Scan Devices",
		GuiRefreshStatus:      "📊 Refresh Status",
		GuiDiscovering:        "Scanning...",
		GuiDevicesFound:       "Found %d device(s):\n%s",
		GuiNoDevicesFound:     "No ESP32 found.\nMake sure it is on the same WiFi network.",
		GuiCaptureSent:        "Remote Capture",
		GuiCaptureQueued:      "Capture command sent. The photo will arrive in a few seconds.",
		GuiCaptureFailed:      "Capture Failed",
		GuiIPRequired:         "Please enter the ESP32 IP address first.",
		GuiSecretRequired:     "Please enter the device secret first.",
		GuiIntervalSendFailed: "Failed to send interval to device",
		GuiNotConnected:       "Not connected",
		GuiOfflineUnreachable: "Offline or unreachable",
		GuiSignal:             "Signal: %d dBm | Uptime: %ds | Firmware: %s",
		GuiFreeHeap:           "Free heap: %d KB",
		GuiScheduleInvalid:    "Enter a valid delay in seconds (1-86400).",
		GuiScheduled:          "Scheduled",
		ErrorUpload:           "Upload failed",
		ErrorNotFound:         "Not found",
		ErrorUnauthorized:     "Unauthorized",
	},
	LangZhCN: {
		Title:                 "生物成长记录仪",
		Subtitle:              "照片画廊",
		Loading:               "加载中...",
		Refresh:               "刷新",
		Close:                 "关闭",
		Delete:                "删除",
		Confirm:               "确认",
		Cancel:                "取消",
		TotalPhotos:           "张照片",
		DeviceCount:           "台设备",
		TotalSize:             "MB 存储",
		ServerTime:            "服务器时间",
		NoPhotos:              "暂无照片",
		WaitingPhotos:         "等待 ESP32 推送照片中...",
		UploadAddress:         "ESP32 上传地址",
		Device:                "设备",
		Photos:                "张照片",
		Date:                  "日期",
		PlayTimelapse:         "播放延时",
		DownloadMP4:           "下载MP4",
		PrevPhoto:             "上一张",
		NextPhoto:             "下一张",
		Generating:            "生成中...",
		PlayingTimelapse:      "延时播放中... %d 张照片 @ 24fps",
		Login:                 "登录",
		Password:              "密码",
		Logout:                "退出登录",
		LoginRequired:         "需要登录才能查看画廊",
		WrongPassword:         "密码错误",
		Devices:               "设备管理",
		RegisterDevice:        "注册设备",
		DeviceID:              "设备ID",
		DeviceSecret:          "设备密钥(十六进制)",
		DeviceName:            "设备名称",
		Status:                "状态",
		LastSeen:              "最后活跃",
		Retention:             "保留策略",
		PhotoMode:             "拍照模式",
		PhotoInterval:         "拍照间隔",
		Minutes:               "分钟",
		Seconds:               "秒",
		SaveFolder:            "保存文件夹",
		Browse:                "浏览",
		SaveSettings:          "保存设置",
		SettingsSaved:         "设置已保存成功",
		PhotoModeSettings:     "拍照模式设置",
		CustomFolder:          "自定义文件夹",
		DefaultFolder:         "默认文件夹",
		CurrentInterval:       "当前间隔",
		IntervalHint:          "设置摄像头拍照频率（例如 1分钟30秒 = 每90秒拍一张）",
		FolderHint:            "选择接收到的照片在本机保存的位置",
		ApplyToAll:            "应用到所有设备",
		SettingsDesc:          "配置拍照计划和照片存储位置",
		GuiTitle:              "生物成长记录仪 - 存储服务器",
		GuiDashboard:          "仪表盘",
		GuiSettings:           "设置",
		GuiLogs:               "日志",
		GuiStartServer:        "启动服务器",
		GuiStopServer:         "停止服务器",
		GuiRestartServer:      "重启服务器",
		GuiOpenGallery:        "打开 Web 画廊",
		GuiServerStatus:       "服务器状态",
		GuiAccessURL:          "访问地址:",
		GuiUptime:             "运行时间:",
		GuiTotalPhotos:        "照片总数:",
		GuiStorageUsed:        "存储占用:",
		GuiDeviceStatus:       "ESP32 设备状态",
		GuiRunning:            "● 运行中",
		GuiStopped:            "● 已停止",
		GuiStarting:           "● 启动中...",
		GuiStopping:           "● 停止中...",
		GuiListenPort:         "监听端口:",
		GuiDefaultLang:        "默认语言:",
		GuiPhotoStorage:       "照片存储",
		GuiSaveFolder:         "保存文件夹:",
		GuiBrowse:             "浏览...",
		GuiPhotoInterval:      "拍照间隔:",
		GuiApplyToAll:         "应用到所有设备",
		GuiRetentionPolicy:    "保留策略",
		GuiRetentionDays:      "保留天数 (0=永久):",
		GuiMaxStorage:         "最大存储 MB (0=不限):",
		GuiAdmin:              "管理员",
		GuiNewPassword:        "新密码:",
		GuiChangePassword:     "修改密码",
		GuiSaveSettings:       "保存设置",
		GuiClearLogs:          "清空日志",
		GuiAutoScroll:         "自动滚动",
		GuiLanguage:           "语言:",
		GuiConfirmRestart:     "确认重启",
		GuiConfirmRestartMsg:  "重启服务器将短暂中断服务, 确定继续?",
		GuiRestartSuccess:     "重启成功",
		GuiSaveSuccess:        "保存成功",
		GuiSaveSuccessMsg:     "设置已保存。端口和语言更改需要重启服务器生效。",
		GuiStartFailed:        "服务器启动失败",
		GuiPasswordChanged:    "密码已修改",
		GuiPasswordEmpty:      "密码不能为空",
		GuiOnline:             "● 在线",
		GuiOffline:            "● 离线",
		GuiNever:              "从未",
		GuiUnknown:            "未知",
		GuiFirmwareVer:        "固件版本",
		GuiPhotoCount:         "照片数",
		GuiDeviceName:         "名称",
		GuiESP32Setup:         "ESP32 设置向导",
		GuiESP32Step1:         "1. 给 ESP32 通电, 找到 WiFi 网络: BioRecorder-XXXX",
		GuiESP32Step2:         "2. 连接该网络, 打开 192.168.4.1, 输入 WiFi 密码和下方服务器地址",
		GuiESP32Step3:         "3. ESP32 将自动连接 WiFi 并开始上传照片",
		GuiServerForESP32:     "ESP32 服务器地址:",
		GuiSoftAPHint:         "热点名: BioRecorder-XXXX (XXXX = MAC 后4位)",
		GuiWaitingESP32:       "等待 ESP32 连接中...",
		GuiCheckDevices:       "检查已连接设备",
		GuiNoDevicesYet:       "暂无 ESP32 连接, 请按上方步骤操作",
		GuiCopyURL:            "复制地址",
		GuiURLCopied:          "地址已复制到剪贴板",
		GuiESP32Control:       "ESP32 远程控制",
		GuiDeviceIP:           "设备 IP:",
		GuiDeviceSecretLabel:  "设备密钥:",
		GuiRemoteCapture:      "📷 远程拍照",
		GuiCaptureOnce:        "⏱️ 定时拍照一次",
		GuiScheduleCapture:    "计划单次拍照",
		GuiDelaySeconds:       "延迟时间(秒):",
		GuiScanDevices:        "🔍 扫描设备",
		GuiRefreshStatus:      "📊 刷新状态",
		GuiDiscovering:        "扫描中...",
		GuiDevicesFound:       "发现 %d 台设备:\n%s",
		GuiNoDevicesFound:     "未发现 ESP32 设备\n请确认设备已连接同一 WiFi",
		GuiCaptureSent:        "远程拍照",
		GuiCaptureQueued:      "拍照命令已发送, 照片将在数秒内到达",
		GuiCaptureFailed:      "拍照失败",
		GuiIPRequired:         "请先输入 ESP32 IP 地址",
		GuiSecretRequired:     "请先输入设备密钥",
		GuiIntervalSendFailed: "拍照间隔下发失败",
		GuiNotConnected:       "未连接",
		GuiOfflineUnreachable: "离线或无法连接",
		GuiSignal:             "信号: %d dBm | 运行: %ds | 固件: %s",
		GuiFreeHeap:           "空闲内存: %d KB",
		GuiScheduleInvalid:    "请输入有效延迟秒数 (1-86400)",
		GuiScheduled:          "已计划",
		ErrorUpload:           "上传失败",
		ErrorNotFound:         "未找到",
		ErrorUnauthorized:     "未授权",
	},
	LangZhTW: {
		Title:                 "生物成長記錄儀",
		Subtitle:              "照片畫廊",
		Loading:               "載入中...",
		Refresh:               "重新整理",
		Close:                 "關閉",
		Delete:                "刪除",
		Confirm:               "確認",
		Cancel:                "取消",
		TotalPhotos:           "張照片",
		DeviceCount:           "台裝置",
		TotalSize:             "MB 儲存",
		ServerTime:            "伺服器時間",
		NoPhotos:              "暫無照片",
		WaitingPhotos:         "等待 ESP32 推送照片中...",
		UploadAddress:         "ESP32 上傳位址",
		Device:                "裝置",
		Photos:                "張照片",
		Date:                  "日期",
		PlayTimelapse:         "播放縮時",
		DownloadMP4:           "下載MP4",
		PrevPhoto:             "上一張",
		NextPhoto:             "下一張",
		Generating:            "產生中...",
		PlayingTimelapse:      "縮時播放中... %d 張照片 @ 24fps",
		Login:                 "登入",
		Password:              "密碼",
		Logout:                "登出",
		LoginRequired:         "需要登入才能檢視畫廊",
		WrongPassword:         "密碼錯誤",
		Devices:               "裝置管理",
		RegisterDevice:        "註冊裝置",
		DeviceID:              "裝置ID",
		DeviceSecret:          "裝置金鑰(十六進位)",
		DeviceName:            "裝置名稱",
		Status:                "狀態",
		LastSeen:              "最後活躍",
		Retention:             "保留策略",
		PhotoMode:             "拍照模式",
		PhotoInterval:         "拍照間隔",
		Minutes:               "分鐘",
		Seconds:               "秒",
		SaveFolder:            "儲存資料夾",
		Browse:                "瀏覽",
		SaveSettings:          "儲存設定",
		SettingsSaved:         "設定已儲存成功",
		PhotoModeSettings:     "拍照模式設定",
		CustomFolder:          "自訂資料夾",
		DefaultFolder:         "預設資料夾",
		CurrentInterval:       "目前間隔",
		IntervalHint:          "設定攝影機拍照頻率（例如 1分鐘30秒 = 每90秒拍一張）",
		FolderHint:            "選擇接收到的照片在本機儲存的位置",
		ApplyToAll:            "套用至所有裝置",
		SettingsDesc:          "設定拍照計畫和照片儲存位置",
		GuiTitle:              "生物成長記錄儀 - 儲存伺服器",
		GuiDashboard:          "儀表板",
		GuiSettings:           "設定",
		GuiLogs:               "日誌",
		GuiStartServer:        "啟動伺服器",
		GuiStopServer:         "停止伺服器",
		GuiRestartServer:      "重啟伺服器",
		GuiOpenGallery:        "開啟 Web 畫廊",
		GuiServerStatus:       "伺服器狀態",
		GuiAccessURL:          "存取位址:",
		GuiUptime:             "運行時間:",
		GuiTotalPhotos:        "照片總數:",
		GuiStorageUsed:        "儲存佔用:",
		GuiDeviceStatus:       "ESP32 裝置狀態",
		GuiRunning:            "● 運行中",
		GuiStopped:            "● 已停止",
		GuiStarting:           "● 啟動中...",
		GuiStopping:           "● 停止中...",
		GuiListenPort:         "監聽連接埠:",
		GuiDefaultLang:        "預設語言:",
		GuiPhotoStorage:       "照片儲存",
		GuiSaveFolder:         "儲存資料夾:",
		GuiBrowse:             "瀏覽...",
		GuiPhotoInterval:      "拍照間隔:",
		GuiApplyToAll:         "套用至所有裝置",
		GuiRetentionPolicy:    "保留策略",
		GuiRetentionDays:      "保留天數 (0=永久):",
		GuiMaxStorage:         "最大儲存 MB (0=不限):",
		GuiAdmin:              "管理員",
		GuiNewPassword:        "新密碼:",
		GuiChangePassword:     "修改密碼",
		GuiSaveSettings:       "儲存設定",
		GuiClearLogs:          "清空日誌",
		GuiAutoScroll:         "自動捲動",
		GuiLanguage:           "語言:",
		GuiConfirmRestart:     "確認重啟",
		GuiConfirmRestartMsg:  "重啟伺服器將短暫中斷服務, 確定繼續?",
		GuiRestartSuccess:     "重啟成功",
		GuiSaveSuccess:        "儲存成功",
		GuiSaveSuccessMsg:     "設定已儲存。連接埠和語言變更需要重啟伺服器生效。",
		GuiStartFailed:        "伺服器啟動失敗",
		GuiPasswordChanged:    "密碼已修改",
		GuiPasswordEmpty:      "密碼不能為空",
		GuiOnline:             "● 上線",
		GuiOffline:            "● 離線",
		GuiNever:              "從未",
		GuiUnknown:            "未知",
		GuiFirmwareVer:        "韌體版本",
		GuiPhotoCount:         "照片數",
		GuiDeviceName:         "名稱",
		GuiESP32Setup:         "ESP32 設定精靈",
		GuiESP32Step1:         "1. 給 ESP32 通電, 找到 WiFi 網路: BioRecorder-XXXX",
		GuiESP32Step2:         "2. 連接該網路, 開啟 192.168.4.1, 輸入 WiFi 密碼和下方伺服器位址",
		GuiESP32Step3:         "3. ESP32 將自動連接 WiFi 並開始上傳照片",
		GuiServerForESP32:     "ESP32 伺服器位址:",
		GuiSoftAPHint:         "熱點名: BioRecorder-XXXX (XXXX = MAC 後4位)",
		GuiWaitingESP32:       "等待 ESP32 連接中...",
		GuiCheckDevices:       "檢查已連接裝置",
		GuiNoDevicesYet:       "暫無 ESP32 連接, 請按上方步驟操作",
		GuiCopyURL:            "複製位址",
		GuiURLCopied:          "位址已複製到剪貼簿",
		GuiESP32Control:       "ESP32 遠端控制",
		GuiDeviceIP:           "設備 IP:",
		GuiDeviceSecretLabel:  "設備密鑰:",
		GuiRemoteCapture:      "📷 遠端拍照",
		GuiCaptureOnce:        "⏱️ 定時拍照一次",
		GuiScheduleCapture:    "排程單次拍照",
		GuiDelaySeconds:       "延遲時間(秒):",
		GuiScanDevices:        "🔍 掃描設備",
		GuiRefreshStatus:      "📊 重新整理狀態",
		GuiDiscovering:        "掃描中...",
		GuiDevicesFound:       "發現 %d 台設備:\n%s",
		GuiNoDevicesFound:     "未發現 ESP32 設備\n請確認設備已連接同一 WiFi",
		GuiCaptureSent:        "遠端拍照",
		GuiCaptureQueued:      "拍照命令已傳送, 照片將在數秒內送達",
		GuiCaptureFailed:      "拍照失敗",
		GuiIPRequired:         "請先輸入 ESP32 IP 位址",
		GuiSecretRequired:     "請先輸入設備密鑰",
		GuiIntervalSendFailed: "拍照間隔下發失敗",
		GuiNotConnected:       "未連線",
		GuiOfflineUnreachable: "離線或無法連線",
		GuiSignal:             "訊號: %d dBm | 運行: %ds | 韌體: %s",
		GuiFreeHeap:           "可用記憶體: %d KB",
		GuiScheduleInvalid:    "請輸入有效延遲秒數 (1-86400)",
		GuiScheduled:          "已排程",
		ErrorUpload:           "上傳失敗",
		ErrorNotFound:         "未找到",
		ErrorUnauthorized:     "未授權",
	},
	LangJa: {
		Title:                 "バイオ成長記録計",
		Subtitle:              "写真ギャラリー",
		Loading:               "読み込み中...",
		Refresh:               "更新",
		Close:                 "閉じる",
		Delete:                "削除",
		Confirm:               "確認",
		Cancel:                "キャンセル",
		TotalPhotos:           "枚の写真",
		DeviceCount:           "台のデバイス",
		TotalSize:             "MB ストレージ",
		ServerTime:            "サーバー時間",
		NoPhotos:              "写真がありません",
		WaitingPhotos:         "ESP32のアップロードを待っています...",
		UploadAddress:         "ESP32アップロードURL",
		Device:                "デバイス",
		Photos:                "枚の写真",
		Date:                  "日付",
		PlayTimelapse:         "タイムラプス再生",
		DownloadMP4:           "MP4ダウンロード",
		PrevPhoto:             "前へ",
		NextPhoto:             "次へ",
		Generating:            "生成中...",
		PlayingTimelapse:      "タイムラプス再生中... %d枚 @ 24fps",
		Login:                 "ログイン",
		Password:              "パスワード",
		Logout:                "ログアウト",
		LoginRequired:         "ギャラリーの表示にはログインが必要です",
		WrongPassword:         "パスワードが間違っています",
		Devices:               "デバイス管理",
		RegisterDevice:        "デバイス登録",
		DeviceID:              "デバイスID",
		DeviceSecret:          "デバイスシークレット(16進数)",
		DeviceName:            "デバイス名",
		Status:                "ステータス",
		LastSeen:              "最終アクセス",
		Retention:             "保持ポリシー",
		PhotoMode:             "撮影モード",
		PhotoInterval:         "撮影間隔",
		Minutes:               "分",
		Seconds:               "秒",
		SaveFolder:            "保存フォルダ",
		Browse:                "参照",
		SaveSettings:          "設定を保存",
		SettingsSaved:         "設定が保存されました",
		PhotoModeSettings:     "撮影モード設定",
		CustomFolder:          "カスタムフォルダ",
		DefaultFolder:         "デフォルトフォルダ",
		CurrentInterval:       "現在の間隔",
		IntervalHint:          "カメラの撮影頻度を設定（例：1分30秒 = 90秒ごと）",
		FolderHint:            "受信した写真の保存先を選択",
		ApplyToAll:            "全デバイスに適用",
		SettingsDesc:          "撮影スケジュールと保存場所を設定",
		GuiTitle:              "バイオ成長記録計 - ストレージサーバー",
		GuiDashboard:          "ダッシュボード",
		GuiSettings:           "設定",
		GuiLogs:               "ログ",
		GuiStartServer:        "サーバー起動",
		GuiStopServer:         "サーバー停止",
		GuiRestartServer:      "サーバー再起動",
		GuiOpenGallery:        "Webギャラリーを開く",
		GuiServerStatus:       "サーバーステータス",
		GuiAccessURL:          "アクセスURL:",
		GuiUptime:             "稼働時間:",
		GuiTotalPhotos:        "写真総数:",
		GuiStorageUsed:        "ストレージ使用量:",
		GuiDeviceStatus:       "ESP32 デバイス状態",
		GuiRunning:            "● 実行中",
		GuiStopped:            "● 停止",
		GuiStarting:           "● 開始中...",
		GuiStopping:           "● 停止中...",
		GuiListenPort:         "リッスンポート:",
		GuiDefaultLang:        "デフォルト言語:",
		GuiPhotoStorage:       "写真ストレージ",
		GuiSaveFolder:         "保存フォルダ:",
		GuiBrowse:             "参照...",
		GuiPhotoInterval:      "撮影間隔:",
		GuiApplyToAll:         "全デバイスに適用",
		GuiRetentionPolicy:    "保持ポリシー",
		GuiRetentionDays:      "保持日数 (0=無期限):",
		GuiMaxStorage:         "最大ストレージ MB (0=無制限):",
		GuiAdmin:              "管理者",
		GuiNewPassword:        "新しいパスワード:",
		GuiChangePassword:     "パスワード変更",
		GuiSaveSettings:       "設定を保存",
		GuiClearLogs:          "ログを消去",
		GuiAutoScroll:         "自動スクロール",
		GuiLanguage:           "言語:",
		GuiConfirmRestart:     "再起動の確認",
		GuiConfirmRestartMsg:  "サーバーを再起動するとサービスが一時的に中断されます。続行しますか？",
		GuiRestartSuccess:     "再起動成功",
		GuiSaveSuccess:        "保存成功",
		GuiSaveSuccessMsg:     "設定が保存されました。ポートと言語の変更にはサーバーの再起動が必要です。",
		GuiStartFailed:        "サーバーの起動に失敗しました",
		GuiPasswordChanged:    "パスワードが変更されました",
		GuiPasswordEmpty:      "パスワードは空にできません",
		GuiOnline:             "● オンライン",
		GuiOffline:            "● オフライン",
		GuiNever:              "なし",
		GuiUnknown:            "不明",
		GuiFirmwareVer:        "ファームウェア",
		GuiPhotoCount:         "写真数",
		GuiDeviceName:         "名前",
		GuiESP32Setup:         "ESP32 セットアップガイド",
		GuiESP32Step1:         "1. ESP32 の電源を入れ、WiFi ネットワーク BioRecorder-XXXX を探す",
		GuiESP32Step2:         "2. 接続して 192.168.4.1 を開き、WiFi と下記サーバー URL を入力",
		GuiESP32Step3:         "3. ESP32 が自動接続し、写真のアップロードを開始します",
		GuiServerForESP32:     "ESP32 用サーバー URL:",
		GuiSoftAPHint:         "SSID: BioRecorder-XXXX (XXXX = MAC の下4桁)",
		GuiWaitingESP32:       "ESP32 の接続を待っています...",
		GuiCheckDevices:       "接続済みデバイスを確認",
		GuiNoDevicesYet:       "ESP32 がまだ接続されていません。上記の手順に従ってください。",
		GuiCopyURL:            "URL をコピー",
		GuiURLCopied:          "URL をクリップボードにコピーしました",
		GuiESP32Control:       "ESP32 リモートコントロール",
		GuiDeviceIP:           "デバイス IP:",
		GuiDeviceSecretLabel:  "デバイスシークレット:",
		GuiRemoteCapture:      "📷 撮影する",
		GuiCaptureOnce:        "⏱️ 一度だけタイマー撮影",
		GuiScheduleCapture:    "一度だけ撮影を予約",
		GuiDelaySeconds:       "遅延時間(秒):",
		GuiScanDevices:        "🔍 デバイスをスキャン",
		GuiRefreshStatus:      "📊 ステータス更新",
		GuiDiscovering:        "スキャン中...",
		GuiDevicesFound:       "%d 台のデバイスを検出:\n%s",
		GuiNoDevicesFound:     "ESP32 が見つかりません\n同じ WiFi に接続してください",
		GuiCaptureSent:        "リモート撮影",
		GuiCaptureQueued:      "撮影コマンドを送信しました。数秒で写真が届きます。",
		GuiCaptureFailed:      "撮影に失敗しました",
		GuiIPRequired:         "ESP32 の IP アドレスを入力してください",
		GuiSecretRequired:     "デバイスシークレットを入力してください",
		GuiIntervalSendFailed: "撮影間隔をデバイスに送信できませんでした",
		GuiNotConnected:       "未接続",
		GuiOfflineUnreachable: "オフラインまたは接続できません",
		GuiSignal:             "信号: %d dBm | 稼働: %ds | ファームウェア: %s",
		GuiFreeHeap:           "空きメモリ: %d KB",
		GuiScheduleInvalid:    "有効な秒数を入力してください (1-86400)",
		GuiScheduled:          "予約済み",
		ErrorUpload:           "アップロード失敗",
		ErrorNotFound:         "見つかりません",
		ErrorUnauthorized:     "認証エラー",
	},
	LangEs: {
		Title:                 "Registrador de Crecimiento Biológico",
		Subtitle:              "Galería de Fotos",
		Loading:               "Cargando...",
		Refresh:               "Actualizar",
		Close:                 "Cerrar",
		Delete:                "Eliminar",
		Confirm:               "Confirmar",
		Cancel:                "Cancelar",
		TotalPhotos:           "Fotos",
		DeviceCount:           "Dispositivos",
		TotalSize:             "MB Almacenamiento",
		ServerTime:            "Hora del Servidor",
		NoPhotos:              "Sin fotos aún",
		WaitingPhotos:         "Esperando que ESP32 suba fotos...",
		UploadAddress:         "URL de carga ESP32",
		Device:                "Dispositivo",
		Photos:                "fotos",
		Date:                  "Fecha",
		PlayTimelapse:         "Reproducir Lapso de Tiempo",
		DownloadMP4:           "Descargar MP4",
		PrevPhoto:             "Anterior",
		NextPhoto:             "Siguiente",
		Generating:            "Generando...",
		PlayingTimelapse:      "Reproduciendo lapso... %d fotos @ 24fps",
		Login:                 "Iniciar sesión",
		Password:              "Contraseña",
		Logout:                "Cerrar sesión",
		LoginRequired:         "Se requiere inicio de sesión para ver la galería",
		WrongPassword:         "Contraseña incorrecta",
		Devices:               "Gestión de Dispositivos",
		RegisterDevice:        "Registrar Dispositivo",
		DeviceID:              "ID del Dispositivo",
		DeviceSecret:          "Secreto del Dispositivo (hex)",
		DeviceName:            "Nombre del Dispositivo",
		Status:                "Estado",
		LastSeen:              "Última Actividad",
		Retention:             "Retención",
		PhotoMode:             "Modo de Foto",
		PhotoInterval:         "Intervalo de Foto",
		Minutes:               "min",
		Seconds:               "seg",
		SaveFolder:            "Carpeta de Guardado",
		Browse:                "Examinar",
		SaveSettings:          "Guardar Configuración",
		SettingsSaved:         "Configuración guardada con éxito",
		PhotoModeSettings:     "Configuración de Modo de Foto",
		CustomFolder:          "Carpeta Personalizada",
		DefaultFolder:         "Carpeta Predeterminada",
		CurrentInterval:       "Intervalo Actual",
		IntervalHint:          "Configura la frecuencia de toma de fotos (ej. 1 min 30 seg = cada 90 segundos)",
		FolderHint:            "Elige dónde se guardan las fotos recibidas en esta computadora",
		ApplyToAll:            "Aplicar a todos los dispositivos",
		SettingsDesc:          "Configura el horario de captura y la ubicación de almacenamiento",
		GuiTitle:              "Registrador de Crecimiento Biológico - Servidor de Almacenamiento",
		GuiDashboard:          "Panel de Control",
		GuiSettings:           "Configuración",
		GuiLogs:               "Registros",
		GuiStartServer:        "Iniciar Servidor",
		GuiStopServer:         "Detener Servidor",
		GuiRestartServer:      "Reiniciar Servidor",
		GuiOpenGallery:        "Abrir Galería Web",
		GuiServerStatus:       "Estado del Servidor",
		GuiAccessURL:          "URL de Acceso:",
		GuiUptime:             "Tiempo de Actividad:",
		GuiTotalPhotos:        "Total de Fotos:",
		GuiStorageUsed:        "Almacenamiento Usado:",
		GuiDeviceStatus:       "Estado del Dispositivo ESP32",
		GuiRunning:            "● En Ejecución",
		GuiStopped:            "● Detenido",
		GuiStarting:           "● Iniciando...",
		GuiStopping:           "● Deteniendo...",
		GuiListenPort:         "Puerto de Escucha:",
		GuiDefaultLang:        "Idioma Predeterminado:",
		GuiPhotoStorage:       "Almacenamiento de Fotos",
		GuiSaveFolder:         "Carpeta de Guardado:",
		GuiBrowse:             "Examinar...",
		GuiPhotoInterval:      "Intervalo de Foto:",
		GuiApplyToAll:         "Aplicar a todos los dispositivos",
		GuiRetentionPolicy:    "Política de Retención",
		GuiRetentionDays:      "Días de retención (0=permanente):",
		GuiMaxStorage:         "Almacenamiento máximo MB (0=sin límite):",
		GuiAdmin:              "Administrador",
		GuiNewPassword:        "Nueva Contraseña:",
		GuiChangePassword:     "Cambiar Contraseña",
		GuiSaveSettings:       "Guardar Configuración",
		GuiClearLogs:          "Borrar Registros",
		GuiAutoScroll:         "Desplazamiento Automático",
		GuiLanguage:           "Idioma:",
		GuiConfirmRestart:     "Confirmar Reinicio",
		GuiConfirmRestartMsg:  "Reiniciar el servidor interrumpirá brevemente el servicio. ¿Continuar?",
		GuiRestartSuccess:     "Reinicio Exitoso",
		GuiSaveSuccess:        "Guardado Exitoso",
		GuiSaveSuccessMsg:     "Configuración guardada. Los cambios de puerto e idioma requieren reiniciar el servidor.",
		GuiStartFailed:        "Error al iniciar el servidor",
		GuiPasswordChanged:    "Contraseña Cambiada",
		GuiPasswordEmpty:      "La contraseña no puede estar vacía",
		GuiOnline:             "● En Línea",
		GuiOffline:            "● Sin Conexión",
		GuiNever:              "Nunca",
		GuiUnknown:            "Desconocido",
		GuiFirmwareVer:        "Firmware",
		GuiPhotoCount:         "Fotos",
		GuiDeviceName:         "Nombre",
		GuiESP32Setup:         "Guía de Configuración ESP32",
		GuiESP32Step1:         "1. Encienda el ESP32, busque la red WiFi: BioRecorder-XXXX",
		GuiESP32Step2:         "2. Conéctese, abra 192.168.4.1, ingrese WiFi y la URL del servidor",
		GuiESP32Step3:         "3. El ESP32 se conectará automáticamente y subirá fotos",
		GuiServerForESP32:     "URL del servidor para ESP32:",
		GuiSoftAPHint:         "SSID: BioRecorder-XXXX (XXXX = últimos 4 del MAC)",
		GuiWaitingESP32:       "Esperando conexión del ESP32...",
		GuiCheckDevices:       "Verificar Dispositivos Conectados",
		GuiNoDevicesYet:       "Ningún ESP32 conectado. Siga los pasos anteriores.",
		GuiCopyURL:            "Copiar URL",
		GuiURLCopied:          "URL copiada al portapapeles",
		GuiESP32Control:       "Control Remoto ESP32",
		GuiDeviceIP:           "IP del Dispositivo:",
		GuiDeviceSecretLabel:  "Secreto del Dispositivo:",
		GuiRemoteCapture:      "📷 Capturar Ahora",
		GuiCaptureOnce:        "⏱️ Programar Captura",
		GuiScheduleCapture:    "Programar una captura",
		GuiDelaySeconds:       "Retraso (segundos):",
		GuiScanDevices:        "🔍 Buscar Dispositivos",
		GuiRefreshStatus:      "📊 Actualizar Estado",
		GuiDiscovering:        "Buscando...",
		GuiDevicesFound:       "Se encontraron %d dispositivo(s):\n%s",
		GuiNoDevicesFound:     "No se encontró ESP32.\nConfirma que esté en la misma red WiFi.",
		GuiCaptureSent:        "Captura Remota",
		GuiCaptureQueued:      "Comando enviado. La foto llegará en unos segundos.",
		GuiCaptureFailed:      "Error de Captura",
		GuiIPRequired:         "Primero ingresa la IP del ESP32.",
		GuiSecretRequired:     "Primero ingresa el secreto del dispositivo.",
		GuiIntervalSendFailed: "No se pudo enviar el intervalo al dispositivo",
		GuiNotConnected:       "No conectado",
		GuiOfflineUnreachable: "Sin conexión o inaccesible",
		GuiSignal:             "Señal: %d dBm | Tiempo: %ds | Firmware: %s",
		GuiFreeHeap:           "Memoria libre: %d KB",
		GuiScheduleInvalid:    "Ingresa un retraso válido en segundos (1-86400).",
		GuiScheduled:          "Programado",
		ErrorUpload:           "Error de carga",
		ErrorNotFound:         "No encontrado",
		ErrorUnauthorized:     "No autorizado",
	},
	LangFr: {
		Title:                 "Enregistreur de Croissance Biologique",
		Subtitle:              "Galerie de Photos",
		Loading:               "Chargement...",
		Refresh:               "Actualiser",
		Close:                 "Fermer",
		Delete:                "Supprimer",
		Confirm:               "Confirmer",
		Cancel:                "Annuler",
		TotalPhotos:           "Photos",
		DeviceCount:           "Appareils",
		TotalSize:             "Mo Stockage",
		ServerTime:            "Heure du Serveur",
		NoPhotos:              "Aucune photo",
		WaitingPhotos:         "En attente de photos de l'ESP32...",
		UploadAddress:         "URL de téléchargement ESP32",
		Device:                "Appareil",
		Photos:                "photos",
		Date:                  "Date",
		PlayTimelapse:         "Lire le Timelapse",
		DownloadMP4:           "Télécharger MP4",
		PrevPhoto:             "Précédent",
		NextPhoto:             "Suivant",
		Generating:            "Génération...",
		PlayingTimelapse:      "Lecture timelapse... %d photos @ 24fps",
		Login:                 "Connexion",
		Password:              "Mot de passe",
		Logout:                "Déconnexion",
		LoginRequired:         "Connexion requise pour voir la galerie",
		WrongPassword:         "Mot de passe incorrect",
		Devices:               "Gestion des Appareils",
		RegisterDevice:        "Enregistrer Appareil",
		DeviceID:              "ID de l'Appareil",
		DeviceSecret:          "Secret de l'Appareil (hex)",
		DeviceName:            "Nom de l'Appareil",
		Status:                "Statut",
		LastSeen:              "Dernière Activité",
		Retention:             "Rétention",
		PhotoMode:             "Mode Photo",
		PhotoInterval:         "Intervalle Photo",
		Minutes:               "min",
		Seconds:               "sec",
		SaveFolder:            "Dossier de Sauvegarde",
		Browse:                "Parcourir",
		SaveSettings:          "Enregistrer",
		SettingsSaved:         "Paramètres enregistrés avec succès",
		PhotoModeSettings:     "Paramètres du Mode Photo",
		CustomFolder:          "Dossier Personnalisé",
		DefaultFolder:         "Dossier par Défaut",
		CurrentInterval:       "Intervalle Actuel",
		IntervalHint:          "Définir la fréquence de prise de photo (ex. 1 min 30 sec = toutes les 90 secondes)",
		FolderHint:            "Choisir où les photos reçues sont sauvegardées sur cet ordinateur",
		ApplyToAll:            "Appliquer à tous les appareils",
		SettingsDesc:          "Configurer le planning de capture et l'emplacement de stockage",
		GuiTitle:              "Enregistreur de Croissance Biologique - Serveur de Stockage",
		GuiDashboard:          "Tableau de Bord",
		GuiSettings:           "Paramètres",
		GuiLogs:               "Journaux",
		GuiStartServer:        "Démarrer le Serveur",
		GuiStopServer:         "Arrêter le Serveur",
		GuiRestartServer:      "Redémarrer le Serveur",
		GuiOpenGallery:        "Ouvrir la Galerie Web",
		GuiServerStatus:       "État du Serveur",
		GuiAccessURL:          "URL d'Accès :",
		GuiUptime:             "Temps de Fonctionnement :",
		GuiTotalPhotos:        "Total des Photos :",
		GuiStorageUsed:        "Stockage Utilisé :",
		GuiDeviceStatus:       "État de l'Appareil ESP32",
		GuiRunning:            "● En Cours",
		GuiStopped:            "● Arrêté",
		GuiStarting:           "● Démarrage...",
		GuiStopping:           "● Arrêt...",
		GuiListenPort:         "Port d'Écoute :",
		GuiDefaultLang:        "Langue par Défaut :",
		GuiPhotoStorage:       "Stockage des Photos",
		GuiSaveFolder:         "Dossier de Sauvegarde :",
		GuiBrowse:             "Parcourir...",
		GuiPhotoInterval:      "Intervalle Photo :",
		GuiApplyToAll:         "Appliquer à tous les appareils",
		GuiRetentionPolicy:    "Politique de Rétention",
		GuiRetentionDays:      "Jours de rétention (0=illimité) :",
		GuiMaxStorage:         "Stockage max Mo (0=illimité) :",
		GuiAdmin:              "Administrateur",
		GuiNewPassword:        "Nouveau Mot de Passe :",
		GuiChangePassword:     "Changer le Mot de Passe",
		GuiSaveSettings:       "Enregistrer",
		GuiClearLogs:          "Effacer les Journaux",
		GuiAutoScroll:         "Défilement Automatique",
		GuiLanguage:           "Langue :",
		GuiConfirmRestart:     "Confirmer le Redémarrage",
		GuiConfirmRestartMsg:  "Le redémarrage du serveur interrompra brièvement le service. Continuer ?",
		GuiRestartSuccess:     "Redémarrage Réussi",
		GuiSaveSuccess:        "Enregistrement Réussi",
		GuiSaveSuccessMsg:     "Paramètres enregistrés. Les modifications de port et de langue nécessitent un redémarrage du serveur.",
		GuiStartFailed:        "Échec du démarrage du serveur",
		GuiPasswordChanged:    "Mot de Passe Modifié",
		GuiPasswordEmpty:      "Le mot de passe ne peut pas être vide",
		GuiOnline:             "● En Ligne",
		GuiOffline:            "● Hors Ligne",
		GuiNever:              "Jamais",
		GuiUnknown:            "Inconnu",
		GuiFirmwareVer:        "Firmware",
		GuiPhotoCount:         "Photos",
		GuiDeviceName:         "Nom",
		GuiESP32Setup:         "Guide de Configuration ESP32",
		GuiESP32Step1:         "1. Allumez l'ESP32, cherchez le réseau WiFi : BioRecorder-XXXX",
		GuiESP32Step2:         "2. Connectez-vous, ouvrez 192.168.4.1, entrez WiFi et l'URL du serveur",
		GuiESP32Step3:         "3. L'ESP32 se connectera automatiquement et enverra des photos",
		GuiServerForESP32:     "URL du serveur pour ESP32 :",
		GuiSoftAPHint:         "SSID : BioRecorder-XXXX (XXXX = 4 derniers du MAC)",
		GuiWaitingESP32:       "En attente de connexion ESP32...",
		GuiCheckDevices:       "Vérifier Appareils Connectés",
		GuiNoDevicesYet:       "Aucun ESP32 connecté. Suivez les étapes ci-dessus.",
		GuiCopyURL:            "Copier l'URL",
		GuiURLCopied:          "URL copiée dans le presse-papiers",
		GuiESP32Control:       "Contrôle à Distance ESP32",
		GuiDeviceIP:           "IP de l'Appareil :",
		GuiDeviceSecretLabel:  "Secret de l'Appareil :",
		GuiRemoteCapture:      "📷 Capturer Maintenant",
		GuiCaptureOnce:        "⏱️ Planifier une Capture",
		GuiScheduleCapture:    "Planifier une seule capture",
		GuiDelaySeconds:       "Délai (secondes) :",
		GuiScanDevices:        "🔍 Rechercher",
		GuiRefreshStatus:      "📊 Actualiser",
		GuiDiscovering:        "Recherche...",
		GuiDevicesFound:       "%d appareil(s) trouvé(s) :\n%s",
		GuiNoDevicesFound:     "Aucun ESP32 trouvé.\nVérifiez qu'il est sur le même réseau WiFi.",
		GuiCaptureSent:        "Capture à Distance",
		GuiCaptureQueued:      "Commande envoyée. La photo arrivera dans quelques secondes.",
		GuiCaptureFailed:      "Échec de la Capture",
		GuiIPRequired:         "Veuillez d'abord saisir l'adresse IP de l'ESP32.",
		GuiSecretRequired:     "Veuillez d'abord saisir le secret de l'appareil.",
		GuiIntervalSendFailed: "Échec de l'envoi de l'intervalle à l'appareil",
		GuiNotConnected:       "Non connecté",
		GuiOfflineUnreachable: "Hors ligne ou inaccessible",
		GuiSignal:             "Signal : %d dBm | Durée : %ds | Firmware : %s",
		GuiFreeHeap:           "Mémoire libre : %d KB",
		GuiScheduleInvalid:    "Saisissez un délai valide en secondes (1-86400).",
		GuiScheduled:          "Planifié",
		ErrorUpload:           "Échec du téléchargement",
		ErrorNotFound:         "Introuvable",
		ErrorUnauthorized:     "Non autorisé",
	},
}

// Translate 翻译键到指定语言
func Translate(lang string, key Key) string {
	if t, ok := translations[lang]; ok {
		if v, ok := t[key]; ok {
			return v
		}
	}
	// 回退到英语
	if t, ok := translations[LangEn]; ok {
		if v, ok := t[key]; ok {
			return v
		}
	}
	return string(key)
}

// TranslateF 带格式化参数的翻译
func TranslateF(lang string, key Key, args ...interface{}) string {
	t := Translate(lang, key)
	// 简单的 %d 替换
	for _, arg := range args {
		t = strings.Replace(t, "%d", fmt.Sprintf("%v", arg), 1)
	}
	return t
}

// ParseLang 从 Accept-Language 头解析语言
func ParseLang(acceptLang string, defaultLang string) string {
	if acceptLang == "" {
		return defaultLang
	}
	// 取第一个语言偏好
	parts := strings.Split(acceptLang, ",")
	if len(parts) == 0 {
		return defaultLang
	}
	lang := strings.TrimSpace(strings.Split(parts[0], ";")[0])

	// 精确匹配
	for _, l := range SupportedLanguages {
		if strings.EqualFold(lang, l) {
			return l
		}
	}

	// 前缀匹配 (如 "en-US" -> "en", "zh" -> "zh-CN")
	lower := strings.ToLower(lang)
	switch {
	case strings.HasPrefix(lower, "zh-cn") || strings.HasPrefix(lower, "zh-hans"):
		return LangZhCN
	case strings.HasPrefix(lower, "zh-tw") || strings.HasPrefix(lower, "zh-hant") || strings.HasPrefix(lower, "zh-hk"):
		return LangZhTW
	case strings.HasPrefix(lower, "zh"):
		return LangZhCN
	case strings.HasPrefix(lower, "ja"):
		return LangJa
	case strings.HasPrefix(lower, "es"):
		return LangEs
	case strings.HasPrefix(lower, "fr"):
		return LangFr
	case strings.HasPrefix(lower, "en"):
		return LangEn
	}
	return defaultLang
}
