// ============================================================
//  【电脑端软件】运行目标: Windows / macOS / Linux / NAS 服务器
//  编译工具链: Go 1.21+ (跨平台单二进制, 不依赖 ESP32)
//  运行方式: 直接运行二进制 或 Docker 容器 / systemd 服务
// ============================================================
//
// Package util - 输入验证与路径遍历防护
//
// 提供 deviceID / date / filename 的正则校验, 以及文件路径前缀检查,
// 防止路径遍历攻击 (如 ../../etc/passwd)
package util

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// 预编译正则 (失败时 panic 仅在启动期, 这里不会发生)
var (
	// deviceID: 字母数字下划线连字符, 长度 1-64
	deviceIDRe = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	// date: YYYY-MM-DD
	dateRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	// filename: 字母数字下划线点连字符 + .jpg (大小写不敏感)
	filenameRe = regexp.MustCompile(`(?i)^[\w.-]+\.jpg$`)
)

// ValidateDeviceID 校验设备 ID (字母数字下划线连字符, 长度 1-64)
func ValidateDeviceID(id string) error {
	if len(id) < 1 || len(id) > 64 {
		return fmt.Errorf("设备 ID 长度必须在 1-64 之间")
	}
	if !deviceIDRe.MatchString(id) {
		return fmt.Errorf("设备 ID 只能包含字母、数字、下划线和连字符")
	}
	return nil
}

// ValidateDate 校验日期格式 (YYYY-MM-DD)
func ValidateDate(date string) error {
	if !dateRe.MatchString(date) {
		return fmt.Errorf("日期格式必须为 YYYY-MM-DD")
	}
	return nil
}

// ValidateFilename 校验文件名 (必须为 xxx.jpg, 大小写不敏感)
func ValidateFilename(name string) error {
	if !filenameRe.MatchString(name) {
		return fmt.Errorf("文件名只能包含字母、数字、下划线、点和连字符, 且必须以 .jpg 结尾")
	}
	// 额外禁止含路径分隔符 (正则已排除, 这里双重保险)
	if strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("文件名不能包含路径分隔符")
	}
	return nil
}

// SafePathJoin 安全拼接路径并验证结果未越出 baseDir 范围
// 使用 filepath.Clean 规范化后做前缀检查, 防止路径遍历
// 返回清理后的绝对路径
func SafePathJoin(baseDir string, parts ...string) (string, error) {
	all := append([]string{baseDir}, parts...)
	joined := filepath.Join(all...)
	cleaned := filepath.Clean(joined)

	// 计算基准目录的绝对路径
	baseAbs, err := filepath.Abs(filepath.Clean(baseDir))
	if err != nil {
		return "", fmt.Errorf("解析基准目录失败: %w", err)
	}
	absPath, err := filepath.Abs(cleaned)
	if err != nil {
		return "", fmt.Errorf("解析路径失败: %w", err)
	}

	// 前缀检查: 确保最终路径位于基准目录内
	// 使用带分隔符的检查, 避免 /foo 匹配 /foobar
	if absPath != baseAbs && !strings.HasPrefix(absPath, baseAbs+string(filepath.Separator)) {
		return "", fmt.Errorf("路径遍历检测: 路径越出基准目录")
	}
	return absPath, nil
}

// ValidatePhotoRequest 一次性校验 deviceID / date / filename, 用于照片相关 handler
func ValidatePhotoRequest(deviceID, date, filename string) error {
	if err := ValidateDeviceID(deviceID); err != nil {
		return err
	}
	if err := ValidateDate(date); err != nil {
		return err
	}
	if filename != "" {
		if err := ValidateFilename(filename); err != nil {
			return err
		}
	}
	return nil
}
