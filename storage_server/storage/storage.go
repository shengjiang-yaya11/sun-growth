// ============================================================
//  【电脑端软件】运行目标: Windows / macOS / Linux / NAS 服务器
//  编译工具链: Go 1.21+ (跨平台单二进制, 不依赖 ESP32)
//  运行方式: 直接运行二进制 或 Docker 容器 / systemd 服务
// ============================================================
//
// Package storage - 照片存储管理
package storage

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Storage 照片存储管理器
type Storage struct {
	baseDir string
	mu      sync.RWMutex
}

// New 创建存储管理器
func New(baseDir string) (*Storage, error) {
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("创建存储目录失败: %w", err)
	}
	return &Storage{baseDir: baseDir}, nil
}

// BaseDir 返回存储根目录
func (s *Storage) BaseDir() string {
	return s.baseDir
}

// DeleteDevice 安全删除某设备的整个目录
func (s *Storage) DeleteDevice(deviceID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	safeID := sanitizeID(deviceID)
	dir := filepath.Join(s.baseDir, safeID)

	// 验证路径不越界
	absPath, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	baseAbs, _ := filepath.Abs(s.baseDir)
	if !strings.HasPrefix(absPath, baseAbs+string(filepath.Separator)) {
		return fmt.Errorf("path traversal detected")
	}

	if !fileExists(absPath) {
		return nil
	}
	return os.RemoveAll(absPath)
}

// SavePhoto 保存照片
// 按设备/日期分目录: baseDir/deviceID/2026-07-29/20260729_103000.jpg
func (s *Storage) SavePhoto(deviceID string, data []byte) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 安全化设备 ID (防止路径遍历)
	safeID := sanitizeID(deviceID)
	now := time.Now()
	dateStr := now.Format("2006-01-02")

	dir := filepath.Join(s.baseDir, safeID, dateStr)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("创建目录失败: %w", err)
	}

	// 生成文件名
	filename := now.Format("20060102_150405") + ".jpg"
	path := filepath.Join(dir, filename)

	// 同秒冲突处理
	counter := 1
	for fileExists(path) {
		filename = now.Format("20060102_150405") + fmt.Sprintf("_%d.jpg", counter)
		path = filepath.Join(dir, filename)
		counter++
	}

	// 写入文件
	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", fmt.Errorf("写入文件失败: %w", err)
	}

	// 返回相对路径
	relPath, _ := filepath.Rel(s.baseDir, path)
	return relPath, nil
}

// GetPhotoPath 获取照片绝对路径
func (s *Storage) GetPhotoPath(deviceID, dateStr, filename string) (string, error) {
	safeID := sanitizeID(deviceID)
	path := filepath.Join(s.baseDir, safeID, dateStr, filename)

	// 验证路径不越界
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	baseAbs, _ := filepath.Abs(s.baseDir)
	if !strings.HasPrefix(absPath, baseAbs) {
		return "", fmt.Errorf("path traversal detected")
	}

	if !fileExists(absPath) {
		return "", fmt.Errorf("photo not found")
	}

	return absPath, nil
}

// PhotoInfo 照片信息
type PhotoInfo struct {
	Filename string `json:"filename"`
	Time     string `json:"time"`
	Path     string `json:"path"`
}

// DayInfo 某天的照片信息
type DayInfo struct {
	Date   string     `json:"date"`
	Count  int        `json:"count"`
	Photos []PhotoInfo `json:"photos"`
}

// DeviceInfo 设备信息
type DeviceInfo struct {
	DeviceID string   `json:"device_id"`
	Days     []DayInfo `json:"days"`
	Total    int      `json:"total"`
}

// ScanAll 扫描所有照片
func (s *Storage) ScanAll() ([]DeviceInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var devices []DeviceInfo

	entries, err := os.ReadDir(s.baseDir)
	if err != nil {
		return nil, err
	}

	for _, deviceEntry := range entries {
		if !deviceEntry.IsDir() {
			continue
		}
		deviceID := deviceEntry.Name()
		deviceInfo := DeviceInfo{DeviceID: deviceID}

		dateEntries, err := os.ReadDir(filepath.Join(s.baseDir, deviceID))
		if err != nil {
			continue
		}

		for _, dateEntry := range dateEntries {
			if !dateEntry.IsDir() {
				continue
			}
			dateStr := dateEntry.Name()

			photoFiles, err := os.ReadDir(filepath.Join(s.baseDir, deviceID, dateStr))
			if err != nil {
				continue
			}

			var photos []PhotoInfo
			for _, pf := range photoFiles {
				if pf.IsDir() || !isPhotoFile(pf.Name()) {
					continue
				}
				timeStr := extractTime(pf.Name())
				photos = append(photos, PhotoInfo{
					Filename: pf.Name(),
					Time:     timeStr,
					Path:     fmt.Sprintf("/photo/%s/%s/%s", deviceID, dateStr, pf.Name()),
				})
			}

			sort.Slice(photos, func(i, j int) bool {
				return photos[i].Filename < photos[j].Filename
			})

			if len(photos) > 0 {
				deviceInfo.Days = append(deviceInfo.Days, DayInfo{
					Date:   dateStr,
					Count:  len(photos),
					Photos: photos,
				})
				deviceInfo.Total += len(photos)
			}
		}

		// 日期倒序
		sort.Slice(deviceInfo.Days, func(i, j int) bool {
			return deviceInfo.Days[i].Date > deviceInfo.Days[j].Date
		})

		devices = append(devices, deviceInfo)
	}

	return devices, nil
}

// GetStats 获取统计信息
func (s *Storage) GetStats() (totalPhotos int, totalSize int64, deviceCount int) {
	devices, _ := s.ScanAll()
	for _, d := range devices {
		totalPhotos += d.Total
		deviceCount++
	}
	// 计算总大小
	filepath.Walk(s.baseDir, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			totalSize += info.Size()
		}
		return nil
	})
	return
}

// DeleteDay 删除某设备某天的所有照片
func (s *Storage) DeleteDay(deviceID, dateStr string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	safeID := sanitizeID(deviceID)
	dir := filepath.Join(s.baseDir, safeID, dateStr)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, e := range entries {
		if !e.IsDir() {
			os.Remove(filepath.Join(dir, e.Name()))
			count++
		}
	}

	os.Remove(dir) // 删除空目录
	return count, nil
}

// 辅助函数

func sanitizeID(id string) string {
	var sb strings.Builder
	for _, c := range id {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '-' || c == '_' {
			sb.WriteRune(c)
		}
	}
	if sb.Len() == 0 {
		return "unknown"
	}
	return sb.String()
}

func isPhotoFile(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return ext == ".jpg" || ext == ".jpeg"
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func extractTime(filename string) string {
	// 文件名格式: 20260729_103000.jpg 或 20260729_103000_1.jpg
	base := strings.TrimSuffix(filename, filepath.Ext(filename))
	parts := strings.SplitN(base, "_", 2)
	if len(parts) < 2 {
		return ""
	}
	timePart := parts[1]
	if len(timePart) >= 6 {
		return timePart[:2] + ":" + timePart[2:4] + ":" + timePart[4:6]
	}
	return timePart
}

// GetReader 获取照片的读取器
func (s *Storage) GetReader(deviceID, dateStr, filename string) (io.ReadCloser, error) {
	path, err := s.GetPhotoPath(deviceID, dateStr, filename)
	if err != nil {
		return nil, err
	}
	return os.Open(path)
}
