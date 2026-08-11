// ============================================================
//  【电脑端软件】运行目标: Windows / macOS / Linux / NAS 服务器
//  编译工具链: Go 1.21+ (跨平台单二进制, 不依赖 ESP32)
//  运行方式: 直接运行二进制 或 Docker 容器 / systemd 服务
// ============================================================
//
// Package storage - 照片保留策略与存储管理 (ADR-006)
//
// 后台协程每小时检查一次:
//   - 删除超期照片 (超过 retention_days)
//   - 删除超额设备的旧照片 (超过 storage_quota_mb)
//   - 删除超额全局旧照片 (超过 max_storage_mb)
package storage

import (
	"context"
	"log/slog"
	"os"
	"time"

	"bio-growth-recorder/database"
	"bio-growth-recorder/util"
)

// RetentionManager 保留策略管理器
type RetentionManager struct {
	db            *database.DB // 数据库
	baseDir       string       // 照片存储根目录
	retentionDays int          // 保留天数 (0 = 不按时间清理)
	maxStorageMB  int64        // 全局最大存储 (MB, 0 = 不限)
	interval      time.Duration // 检查间隔
}

// NewRetentionManager 创建保留策略管理器
func NewRetentionManager(db *database.DB, baseDir string, retentionDays int, maxStorageMB int64) *RetentionManager {
	return &RetentionManager{
		db:            db,
		baseDir:       baseDir,
		retentionDays: retentionDays,
		maxStorageMB:  maxStorageMB,
		interval:      time.Hour, // 每小时检查一次
	}
}

// Start 启动后台清理协程, 直到 ctx 取消时退出
func (rm *RetentionManager) Start(ctx context.Context) {
	slog.Info("启动照片保留策略清理协程",
		"retention_days", rm.retentionDays,
		"max_storage_mb", rm.maxStorageMB,
		"interval", rm.interval)
	go func() {
		// 启动后先等待一小段时间, 避免与启动流程竞争
		select {
		case <-time.After(10 * time.Second):
		case <-ctx.Done():
			return
		}
		// 首次立即执行一次
		rm.RunOnce()
		ticker := time.NewTicker(rm.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				rm.RunOnce()
			case <-ctx.Done():
				slog.Info("照片保留策略清理协程已停止")
				return
			}
		}
	}()
}

// RunOnce 执行一次清理 (可用于手动触发)
func (rm *RetentionManager) RunOnce() {
	start := time.Now()
	totalDeleted := 0

	// 1. 按保留天数清理超期照片
	if rm.retentionDays > 0 {
		n := rm.cleanExpired()
		totalDeleted += n
		if n > 0 {
			slog.Info("保留策略: 清理超期照片完成", "deleted", n, "retention_days", rm.retentionDays)
		}
	}

	// 2. 按设备配额清理超额设备
	n := rm.cleanDeviceQuotas()
	totalDeleted += n
	if n > 0 {
		slog.Info("保留策略: 清理超额设备照片完成", "deleted", n)
	}

	// 3. 按全局配额清理
	n = rm.cleanGlobalQuota()
	totalDeleted += n
	if n > 0 {
		slog.Info("保留策略: 清理全局超额照片完成", "deleted", n, "max_storage_mb", rm.maxStorageMB)
	}

	if totalDeleted > 0 {
		slog.Info("保留策略清理完成", "total_deleted", totalDeleted, "elapsed", time.Since(start))
	}
}

// cleanExpired 删除超过保留天数的照片, 返回删除数量
func (rm *RetentionManager) cleanExpired() int {
	cutoff := time.Now().Add(-time.Duration(rm.retentionDays) * 24 * time.Hour)
	photos, err := rm.db.GetExpiredPhotos(cutoff)
	if err != nil {
		slog.Error("保留策略: 查询过期照片失败", "error", err)
		return 0
	}
	if len(photos) == 0 {
		return 0
	}
	ids := make([]int64, 0, len(photos))
	for _, p := range photos {
		rm.deletePhotoFile(p.FilePath)
		ids = append(ids, p.PhotoID)
	}
	affected, err := rm.db.DeletePhotosByIDs(ids)
	if err != nil {
		slog.Error("保留策略: 删除过期照片记录失败", "error", err)
		return 0
	}
	return int(affected)
}

// cleanDeviceQuotas 清理超过各自配额的设备旧照片, 返回删除数量
func (rm *RetentionManager) cleanDeviceQuotas() int {
	devices, err := rm.db.ListDevices()
	if err != nil {
		slog.Error("保留策略: 查询设备列表失败", "error", err)
		return 0
	}
	totalDeleted := 0
	for _, dev := range devices {
		if dev.StorageQuotaMB <= 0 {
			continue
		}
		quotaBytes := int64(dev.StorageQuotaMB) * 1024 * 1024
		size, err := rm.db.SumDevicePhotoSize(dev.DeviceID)
		if err != nil {
			slog.Error("保留策略: 统计设备照片大小失败", "device", dev.DeviceID, "error", err)
			continue
		}
		if size <= quotaBytes {
			continue
		}
		// 超额, 按时间升序删除最旧照片
		photos, err := rm.db.GetOldestPhotosByDevice(dev.DeviceID)
		if err != nil {
			slog.Error("保留策略: 查询设备最早照片失败", "device", dev.DeviceID, "error", err)
			continue
		}
		var ids []int64
		freed := int64(0)
		for _, p := range photos {
			if size-freed <= quotaBytes {
				break
			}
			rm.deletePhotoFile(p.FilePath)
			ids = append(ids, p.PhotoID)
			freed += p.FileSize
		}
		if len(ids) > 0 {
			affected, err := rm.db.DeletePhotosByIDs(ids)
			if err != nil {
				slog.Error("保留策略: 删除设备超额照片失败", "device", dev.DeviceID, "error", err)
				continue
			}
			totalDeleted += int(affected)
		}
	}
	return totalDeleted
}

// cleanGlobalQuota 清理超过全局存储配额的旧照片, 返回删除数量
func (rm *RetentionManager) cleanGlobalQuota() int {
	if rm.maxStorageMB <= 0 {
		return 0
	}
	maxBytes := rm.maxStorageMB * 1024 * 1024
	total, err := rm.db.SumPhotoSize()
	if err != nil {
		slog.Error("保留策略: 统计照片总大小失败", "error", err)
		return 0
	}
	if total <= maxBytes {
		return 0
	}
	// 超额, 按时间升序删除全局最旧照片
	photos, err := rm.db.GetOldestPhotosGlobal()
	if err != nil {
		slog.Error("保留策略: 查询全局最早照片失败", "error", err)
		return 0
	}
	var ids []int64
	freed := int64(0)
	for _, p := range photos {
		if total-freed <= maxBytes {
			break
		}
		rm.deletePhotoFile(p.FilePath)
		ids = append(ids, p.PhotoID)
		freed += p.FileSize
	}
	if len(ids) == 0 {
		return 0
	}
	affected, err := rm.db.DeletePhotosByIDs(ids)
	if err != nil {
		slog.Error("保留策略: 删除全局超额照片失败", "error", err)
		return 0
	}
	return int(affected)
}

// deletePhotoFile 删除照片文件 (基于相对路径, 含路径遍历防护)
func (rm *RetentionManager) deletePhotoFile(relPath string) {
	absPath, err := util.SafePathJoin(rm.baseDir, relPath)
	if err != nil {
		slog.Warn("保留策略: 跳过删除文件 (路径不安全)", "path", relPath, "error", err)
		return
	}
	if err := os.Remove(absPath); err != nil && !os.IsNotExist(err) {
		slog.Warn("保留策略: 删除照片文件失败", "path", absPath, "error", err)
	}
}
