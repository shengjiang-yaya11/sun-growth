// ============================================================
//  【电脑端软件】运行目标: Windows / macOS / Linux / NAS 服务器
//  编译工具链: Go 1.21+ (跨平台单二进制, 不依赖 ESP32)
//  运行方式: 直接运行二进制 或 Docker 容器 / systemd 服务
// ============================================================
//
// Package database - 照片元数据 CRUD
package database

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// Photo 照片元数据记录
type Photo struct {
	PhotoID    int64     // 自增主键
	DeviceID   string    // 所属设备 ID
	FilePath   string    // 相对存储路径
	FileSize   int64     // 文件大小 (字节)
	Width      int       // 图片宽度
	Height     int       // 图片高度
	CapturedAt time.Time // 拍摄时间
	ReceivedAt time.Time // 接收时间
	Checksum   string    // 校验和
}

// CreatePhoto 插入一条照片元数据, 返回自增 ID
func (d *DB) CreatePhoto(p *Photo) (int64, error) {
	res, err := d.db.Exec(
		`INSERT INTO photos (device_id, file_path, file_size, width, height, captured_at, received_at, checksum)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		p.DeviceID, p.FilePath, p.FileSize, p.Width, p.Height,
		p.CapturedAt.UTC(), time.Now().UTC(), p.Checksum,
	)
	if err != nil {
		return 0, fmt.Errorf("插入照片记录失败: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("获取照片 ID 失败: %w", err)
	}
	return id, nil
}

// ListPhotosByDevice 分页查询某设备的照片 (按拍摄时间倒序)
func (d *DB) ListPhotosByDevice(deviceID string, limit, offset int) ([]Photo, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := d.db.Query(
		`SELECT photo_id, device_id, file_path, file_size, width, height,
		        captured_at, COALESCE(received_at, ''), checksum
		 FROM photos WHERE device_id = ?
		 ORDER BY captured_at DESC
		 LIMIT ? OFFSET ?`,
		deviceID, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("查询设备照片失败: %w", err)
	}
	defer rows.Close()

	return scanPhotos(rows)
}

// CountPhotosByDevice 统计某设备的照片数量
func (d *DB) CountPhotosByDevice(deviceID string) (int64, error) {
	var count int64
	err := d.db.QueryRow(
		`SELECT COUNT(*) FROM photos WHERE device_id = ?`, deviceID,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("统计设备照片数失败: %w", err)
	}
	return count, nil
}

// CountPhotos 返回照片总数
func (d *DB) CountPhotos() (int64, error) {
	var count int64
	err := d.db.QueryRow(`SELECT COUNT(*) FROM photos`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("统计照片总数失败: %w", err)
	}
	return count, nil
}

// SumPhotoSize 返回所有照片总大小 (字节)
func (d *DB) SumPhotoSize() (int64, error) {
	var total sql.NullInt64
	err := d.db.QueryRow(`SELECT COALESCE(SUM(file_size), 0) FROM photos`).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("统计照片总大小失败: %w", err)
	}
	return total.Int64, nil
}

// SumDevicePhotoSize 返回某设备照片总大小 (字节)
func (d *DB) SumDevicePhotoSize(deviceID string) (int64, error) {
	var total sql.NullInt64
	err := d.db.QueryRow(
		`SELECT COALESCE(SUM(file_size), 0) FROM photos WHERE device_id = ?`, deviceID,
	).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("统计设备照片大小失败: %w", err)
	}
	return total.Int64, nil
}

// GetExpiredPhotos 返回拍摄时间早于 cutoff 的照片 (用于保留策略清理)
func (d *DB) GetExpiredPhotos(cutoff time.Time) ([]Photo, error) {
	rows, err := d.db.Query(
		`SELECT photo_id, device_id, file_path, file_size, width, height,
		        captured_at, COALESCE(received_at, ''), checksum
		 FROM photos WHERE captured_at < ?
		 ORDER BY captured_at ASC`,
		cutoff.UTC(),
	)
	if err != nil {
		return nil, fmt.Errorf("查询过期照片失败: %w", err)
	}
	defer rows.Close()
	return scanPhotos(rows)
}

// GetOldestPhotosByDevice 返回某设备最早的照片 (按拍摄时间升序), 用于配额清理
func (d *DB) GetOldestPhotosByDevice(deviceID string) ([]Photo, error) {
	rows, err := d.db.Query(
		`SELECT photo_id, device_id, file_path, file_size, width, height,
		        captured_at, COALESCE(received_at, ''), checksum
		 FROM photos WHERE device_id = ?
		 ORDER BY captured_at ASC`,
		deviceID,
	)
	if err != nil {
		return nil, fmt.Errorf("查询设备最早照片失败: %w", err)
	}
	defer rows.Close()
	return scanPhotos(rows)
}

// GetOldestPhotosGlobal 返回全局最早的照片 (按拍摄时间升序), 用于全局配额清理
func (d *DB) GetOldestPhotosGlobal() ([]Photo, error) {
	rows, err := d.db.Query(
		`SELECT photo_id, device_id, file_path, file_size, width, height,
		        captured_at, COALESCE(received_at, ''), checksum
		 FROM photos ORDER BY captured_at ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("查询最早照片失败: %w", err)
	}
	defer rows.Close()
	return scanPhotos(rows)
}

// DeletePhotosByIDs 批量删除指定 ID 的照片记录
func (d *DB) DeletePhotosByIDs(ids []int64) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	// 使用参数化查询防止 SQL 注入
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}
	query := fmt.Sprintf(`DELETE FROM photos WHERE photo_id IN (%s)`, strings.Join(placeholders, ","))
	res, err := d.db.Exec(query, args...)
	if err != nil {
		return 0, fmt.Errorf("批量删除照片失败: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("获取删除行数失败: %w", err)
	}
	return affected, nil
}

// DeletePhotosByDevice 删除某设备的所有照片记录
func (d *DB) DeletePhotosByDevice(deviceID string) (int64, error) {
	res, err := d.db.Exec(`DELETE FROM photos WHERE device_id = ?`, deviceID)
	if err != nil {
		return 0, fmt.Errorf("删除设备照片失败: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("获取删除行数失败: %w", err)
	}
	return affected, nil
}

// DeletePhotosByDeviceDate 删除某设备指定日期的照片记录
// dateStr 为 YYYY-MM-DD, 通过匹配 file_path 前缀 (deviceID/dateStr/) 删除
func (d *DB) DeletePhotosByDeviceDate(deviceID, dateStr string) (int64, error) {
	// file_path 格式: deviceID/dateStr/filename.jpg
	// deviceID 与 dateStr 已在上层校验, 不含 LIKE 通配符
	pattern := deviceID + "/" + dateStr + "/%"
	res, err := d.db.Exec(
		`DELETE FROM photos WHERE device_id = ? AND file_path LIKE ?`,
		deviceID, pattern,
	)
	if err != nil {
		return 0, fmt.Errorf("删除设备日期照片失败: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("获取删除行数失败: %w", err)
	}
	return affected, nil
}

// scanPhotos 从 rows 读取照片列表
func scanPhotos(rows *sql.Rows) ([]Photo, error) {
	var photos []Photo
	for rows.Next() {
		var p Photo
		var capturedAt, receivedAt string
		if err := rows.Scan(
			&p.PhotoID, &p.DeviceID, &p.FilePath, &p.FileSize,
			&p.Width, &p.Height, &capturedAt, &receivedAt, &p.Checksum,
		); err != nil {
			return nil, fmt.Errorf("读取照片数据失败: %w", err)
		}
		p.CapturedAt = parseTime(capturedAt)
		p.ReceivedAt = parseTime(receivedAt)
		photos = append(photos, p)
	}
	return photos, rows.Err()
}
