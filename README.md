# ☀️ Sun Growth — ESP32 生物生长记录仪 v3.1

> 双向控制架构: PC 远程控制 ESP32 拍照, 加密回传, 自动存档。

**作者: Andrew 亚生**

---

## 🎯 核心功能

- **双向控制** — PC 端发送命令 (拍照/查状态/改间隔/重启), ESP32 即时响应
- **定时慢拍** — 可配置 X 分钟 X 秒间隔, 自动拍照上传
- **端到端加密** — AES-256-CTR 加密 + HMAC-SHA256 签名, 数据自主可控
- **局域网发现** — PC 自动扫描发现同一 WiFi 下的 ESP32 设备
- **跨平台桌面** — macOS / Windows / Linux GUI + 浏览器 Web 画廊
- **照片管理** — 按设备/日期归档, 延时回放, 自定义保存路径

---

## 🏗️ 系统架构

```
┌─────────────────────┐         WiFi 局域网          ┌──────────────────────────┐
│   ESP32-S3 CAM      │ ←── 命令 (TCP 8081) ────     │   桌面应用 (Go + Fyne)    │
│                     │                              │                          │
│  · 定时拍照          │ ─── 加密照片 (HTTP)  ───>    │  · GUI 窗口控制           │
│  · 命令服务器        │                              │  · 设备发现+状态监控      │
│  · SoftAP 配网       │ ─── 状态响应 (TCP) ────>     │  · 照片存储+画廊          │
│  · AES 加密上传      │                              │  · 延时回放               │
└─────────────────────┘                              └──────────────────────────┘
```

### 命令协议

| 命令 | 方向 | 说明 | 签名 |
|------|------|------|------|
| `capture` | PC → ESP32 | 立即拍照 | HMAC |
| `status` | PC → ESP32 | 查询状态 (RSSI/内存/运行时间) | 公开 |
| `set_config` | PC → ESP32 | 修改拍照间隔 | HMAC |
| `restart` | PC → ESP32 | 重启设备 | HMAC |

---

## 🔧 硬件清单

| 组件 | 型号 |
|------|------|
| 主板 | ESP32-S3 CAM V1.1 |
| 摄像头 | OV3660 (300万像素) |
| Flash | 16MB |
| PSRAM | 8MB |

---

## 🚀 快速开始

### 1. 编译 ESP32 固件

```bash
cd esp32_firmware
./build_firmware.sh     # Docker 编译 (推荐)
./flash_firmware.sh     # 烧录到 ESP32-S3
```

### 2. 配网

1. 上电后 ESP32 开启 **BioRecorder-XXXX** 开放热点
2. 手机/电脑连接此 WiFi
3. 浏览器打开 `http://192.168.4.1`
4. 填写你的 WiFi + 桌面端 IP + 设备 ID + 密钥

### 3. 编译桌面端

```bash
cd storage_server

# macOS
./build_macos_gui.sh

# 全平台
./build_all.sh
```

### 4. 运行

```bash
# GUI 模式
./dist/bio-recorder-darwin-arm64

# 或 Headless 模式 (NAS/Docker)
./dist/bio-recorder-darwin-arm64 --headless
# 浏览器打开 http://localhost:8443
```

---

## 📂 目录结构

```
sun-growth/
├── esp32_firmware/              # ESP32-S3 固件 (C++ / ESP-IDF v5.x)
│   ├── main/
│   │   ├── main.cc              # 主程序 (配网 + 双向控制 + 拍照循环)
│   │   ├── wifi_manager.cc/h    # WiFi STA 连接 (指数退避重连)
│   │   ├── command_server.cc/h  # TCP 命令服务器 (v3.1 新增)
│   │   ├── provisioning.cc/h    # SoftAP Captive Portal 配网
│   │   ├── camera_manager.cc/h  # OV3660 摄像头
│   │   ├── uploader.cc/h        # HTTP 加密上传
│   │   ├── security_bridge.cc/h # mbedTLS 安全模块
│   │   ├── retry_queue.cc/h     # PSRAM 重试队列
│   │   └── config.h             # NVS 键名 + 编译常量
│   ├── components/bio_security/ # 纯 C mbedTLS 安全组件
│   ├── sdkconfig.defaults       # TX 功率 20dBm + Quad PSRAM
│   ├── partitions.csv           # 分区表
│   ├── build_firmware.sh        # Docker 编译
│   └── flash_firmware.sh        # 烧录脚本
│
└── storage_server/              # 跨平台桌面应用 (Go + Fyne)
    ├── main.go                  # HTTP 服务器 + 照片接收
    ├── server_app.go            # 服务器生命周期
    ├── gui_app.go               # GUI 窗口 (仪表盘/设置/ESP32控制)
    ├── device_control.go        # ESP32 双向控制 + 设备发现 (v3.1)
    ├── gui_stub.go              # 无 CGO 自动回退
    ├── headless.go              # 命令行模式
    ├── templates.go             # Web 画廊模板
    ├── config/config.go         # 配置管理
    ├── auth/auth.go             # 设备认证 + AES 解密
    ├── database/                # SQLite (设备/照片/会话)
    ├── storage/                 # 照片存储 + 保留策略
    ├── i18n/i18n.go             # 6 语言国际化
    └── util/                    # JPEG解析 + 路径验证
```

---

## 🔐 安全设计

```
命令签名: HMAC-SHA256(derived_key, cmd||params||timestamp)
密钥派生: key = HMAC(device_secret, "bio-cmd-sig-v1")[:32]

照片加密: AES-256-CTR || HMAC-SHA256 (Encrypt-then-MAC)
密钥派生: key_enc = HMAC(device_secret, "bio-enc-key-v1")[:32]
         key_mac = HMAC(device_secret, "bio-mac-key-v1")[:32]

防重放: 时间戳 5 分钟容差 + Nonce 去重
```
