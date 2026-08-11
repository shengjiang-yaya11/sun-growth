// ============================================================
//  【电脑端软件】运行目标: Windows / macOS / Linux / NAS 服务器
//  编译工具链: Go 1.21+ (跨平台单二进制, 不依赖 ESP32)
//  运行方式: 直接运行二进制 或 Docker 容器 / systemd 服务
// ============================================================
//
// Package database - 设备 CRUD
package database

import (
	"database/sql"
	"fmt"
	"time"
)

// Device 设备记录
type Device struct {
	DeviceID        string         // 设备唯一 ID
	DeviceName      string         // 设备名称
	SecretHex       string         // 设备密钥 (十六进制, 32 字节)
	Status          string         // 状态: active / disabled
	FirmwareVersion string         // 固件版本
	LastSeen        time.Time      // 最后活跃时间
	CreatedAt       time.Time      // 创建时间
	PhotoInterval   int            // 拍照间隔 (秒)
	StorageQuotaMB  int            // 存储配额 (MB), 0 = 不限
}

// CreateDevice 创建新设备
func (d *DB) CreateDevice(deviceID, secretHex, deviceName string) error {
	if deviceName == "" {
		deviceName = deviceID
	}
	_, err := d.db.Exec(
		`INSERT INTO devices (device_id, device_name, secret_hex, status, photo_interval, storage_quota_mb)
		 VALUES (?, ?, ?, 'active', 60, 0)`,
		deviceID, deviceName, secretHex,
	)
	if err != nil {
		return fmt.Errorf("创建设备失败: %w", err)
	}
	return nil
}

// GetDevice 根据 ID 查询单个设备
func (d *DB) GetDevice(deviceID string) (*Device, error) {
	row := d.db.QueryRow(
		`SELECT device_id, device_name, secret_hex, status, firmware_version,
		        COALESCE(last_seen, ''), COALESCE(created_at, ''), photo_interval, storage_quota_mb
		 FROM devices WHERE device_id = ?`,
		deviceID,
	)
	dev, err := scanDevice(row)
	if err != nil {
		return nil, err
	}
	return dev, nil
}

// ListDevices 列出所有设备
func (d *DB) ListDevices() ([]Device, error) {
	rows, err := d.db.Query(
		`SELECT device_id, device_name, secret_hex, status, firmware_version,
		        COALESCE(last_seen, ''), COALESCE(created_at, ''), photo_interval, storage_quota_mb
		 FROM devices ORDER BY created_at ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("查询设备列表失败: %w", err)
	}
	defer rows.Close()

	var devices []Device
	for rows.Next() {
		dev, err := scanDevice(rows)
		if err != nil {
			return nil, err
		}
		devices = append(devices, *dev)
	}
	return devices, rows.Err()
}

// DeleteDevice 删除设备 (关联照片由应用层处理, 表未开启 ON DELETE CASCADE)
func (d *DB) DeleteDevice(deviceID string) error {
	_, err := d.db.Exec(`DELETE FROM devices WHERE device_id = ?`, deviceID)
	if err != nil {
		return fmt.Errorf("删除设备失败: %w", err)
	}
	return nil
}

// UpdateLastSeen 更新设备最后活跃时间
func (d *DB) UpdateLastSeen(deviceID string) error {
	_, err := d.db.Exec(
		`UPDATE devices SET last_seen = ? WHERE device_id = ?`,
		time.Now().UTC(), deviceID,
	)
	if err != nil {
		return fmt.Errorf("更新最后活跃时间失败: %w", err)
	}
	return nil
}

// UpdateFirmwareVersion 更新设备固件版本 (仅在版本变化时写入)
func (d *DB) UpdateFirmwareVersion(deviceID, version string) error {
	if version == "" {
		return nil
	}
	_, err := d.db.Exec(
		`UPDATE devices SET firmware_version = ? WHERE device_id = ? AND (firmware_version != ? OR firmware_version IS NULL OR firmware_version = '')`,
		version, deviceID, version,
	)
	if err != nil {
		return fmt.Errorf("更新固件版本失败: %w", err)
	}
	return nil
}

// UpdatePhotoInterval 更新设备拍照间隔 (秒)
func (d *DB) UpdatePhotoInterval(deviceID string, interval int) error {
	if interval < 1 {
		return fmt.Errorf("拍照间隔不能小于 1 秒")
	}
	_, err := d.db.Exec(
		`UPDATE devices SET photo_interval = ? WHERE device_id = ?`,
		interval, deviceID,
	)
	if err != nil {
		return fmt.Errorf("更新拍照间隔失败: %w", err)
	}
	return nil
}

// UpdateAllPhotoIntervals 更新所有设备的拍照间隔 (秒)
func (d *DB) UpdateAllPhotoIntervals(interval int) error {
	if interval < 1 {
		return fmt.Errorf("拍照间隔不能小于 1 秒")
	}
	_, err := d.db.Exec(`UPDATE devices SET photo_interval = ?`, interval)
	if err != nil {
		return fmt.Errorf("更新所有设备拍照间隔失败: %w", err)
	}
	return nil
}

// CountDevices 返回设备总数
func (d *DB) CountDevices() (int64, error) {
	var count int64
	err := d.db.QueryRow(`SELECT COUNT(*) FROM devices`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("统计设备数失败: %w", err)
	}
	return count, nil
}

// scanner 兼容 *sql.Row 和 *sql.Rows 的 Scan 接口
type scanner interface {
	Scan(dest ...interface{}) error
}

// scanDevice 从 scanner 读取一行设备数据
func scanDevice(s scanner) (*Device, error) {
	var dev Device
	var lastSeen, createdAt string
	err := s.Scan(
		&dev.DeviceID,
		&dev.DeviceName,
		&dev.SecretHex,
		&dev.Status,
		&dev.FirmwareVersion,
		&lastSeen,
		&createdAt,
		&dev.PhotoInterval,
		&dev.StorageQuotaMB,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("设备不存在")
		}
		return nil, fmt.Errorf("读取设备数据失败: %w", err)
	}
	dev.LastSeen = parseTime(lastSeen)
	dev.CreatedAt = parseTime(createdAt)
	return &dev, nil
}

// parseTime 解析 SQLite 返回的时间字符串, 失败返回零值
func parseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	// 尝试多种常见格式
	formats := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		"2006-01-02",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t
		}
	}
	return time.Time{}
}
