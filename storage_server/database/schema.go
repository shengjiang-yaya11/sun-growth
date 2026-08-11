// ============================================================
//  【电脑端软件】运行目标: Windows / macOS / Linux / NAS 服务器
//  编译工具链: Go 1.21+ (跨平台单二进制, 不依赖 ESP32)
//  运行方式: 直接运行二进制 或 Docker 容器 / systemd 服务
// ============================================================
//
// Package database - 表结构定义与迁移
package database

import (
	"fmt"
	"log/slog"
)

// schemaSQL 建表 SQL (基于 Honeydew 语义层设计)
const schemaSQL = `
CREATE TABLE IF NOT EXISTS devices (
    device_id TEXT PRIMARY KEY,
    device_name TEXT NOT NULL DEFAULT '',
    secret_hex TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'active',
    firmware_version TEXT DEFAULT '',
    last_seen DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    photo_interval INTEGER DEFAULT 60,
    storage_quota_mb INTEGER DEFAULT 0
);

CREATE TABLE IF NOT EXISTS photos (
    photo_id INTEGER PRIMARY KEY AUTOINCREMENT,
    device_id TEXT NOT NULL,
    file_path TEXT NOT NULL,
    file_size INTEGER NOT NULL,
    width INTEGER DEFAULT 0,
    height INTEGER DEFAULT 0,
    captured_at DATETIME NOT NULL,
    received_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    checksum TEXT DEFAULT '',
    FOREIGN KEY (device_id) REFERENCES devices(device_id)
);

CREATE TABLE IF NOT EXISTS admin_sessions (
    session_token TEXT PRIMARY KEY,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    expires_at DATETIME NOT NULL,
    ip_address TEXT DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_photos_device ON photos(device_id);
CREATE INDEX IF NOT EXISTS idx_photos_captured ON photos(captured_at);
CREATE INDEX IF NOT EXISTS idx_sessions_expires ON admin_sessions(expires_at);
`

// Migrate 执行数据库表结构迁移 (幂等)
func (d *DB) Migrate() error {
	if _, err := d.db.Exec(schemaSQL); err != nil {
		return fmt.Errorf("执行建表语句失败: %w", err)
	}
	slog.Info("数据库表结构迁移完成")
	return nil
}
