// ============================================================
//  【电脑端软件】运行目标: Windows / macOS / Linux / NAS 服务器
//  编译工具链: Go 1.21+ (跨平台单二进制, 不依赖 ESP32)
//  运行方式: 直接运行二进制 或 Docker 容器 / systemd 服务
// ============================================================
//
// Package config - 配置管理
//
// 支持向后兼容: 旧 config.json 中的明文 admin_password 会自动迁移为 bcrypt 哈希
package config

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// Config 服务器配置
type Config struct {
	// 服务监听地址
	Host string `json:"host"`
	Port int    `json:"port"`

	// 数据存储目录
	DataDir string `json:"data_dir"`

	// SQLite 数据库文件路径
	DBPath string `json:"db_path"`

	// 管理员密码的 bcrypt 哈希 (不再存储明文)
	AdminPasswordHash string `json:"admin_password_hash"`

	// 已注册设备 (设备ID -> 密钥的十六进制)
	// 保留用于向后兼容迁移, 新设备通过 API 写入数据库
	Devices map[string]string `json:"devices"`

	// 默认语言
	DefaultLang string `json:"default_lang"`

	// 时间戳容差 (秒), 用于防重放
	TimestampTolerance int64 `json:"timestamp_tolerance"`

	// 最大上传大小 (字节)
	MaxUploadSize int64 `json:"max_upload_size"`

	// 是否启用 TLS
	TLSEnabled bool   `json:"tls_enabled"`
	TLSCert    string `json:"tls_cert"`
	TLSKey     string `json:"tls_key"`

	// 照片保留天数 (默认 90 天, 0 = 不按时间清理)
	RetentionDays int `json:"retention_days"`

	// 最大存储容量 (MB, 默认 0 = 不限)
	MaxStorageMB int64 `json:"max_storage_mb"`

	// 自定义照片保存文件夹 (空 = 使用 data_dir)
	CustomSaveDir string `json:"custom_save_dir"`
}

// Default 返回默认配置
func Default() *Config {
	return &Config{
		Host:               "0.0.0.0",
		Port:               8443,
		DataDir:            "./captures",
		DBPath:             "./data/bio-recorder.db",
		AdminPasswordHash:  "",
		Devices:            make(map[string]string),
		DefaultLang:        "en",
		TimestampTolerance: 300,              // 5 分钟
		MaxUploadSize:      10 * 1024 * 1024, // 10MB
		TLSEnabled:         false,
		RetentionDays:      90,
		MaxStorageMB:       0,
	}
}

// isBcryptHash 判断字符串是否为 bcrypt 哈希 (以 $2 开头)
func isBcryptHash(s string) bool {
	return strings.HasPrefix(s, "$2")
}

// HashPassword 使用 bcrypt 哈希密码
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("bcrypt 哈希失败: %w", err)
	}
	return string(hash), nil
}

// VerifyPassword 校验明文密码是否匹配哈希
func (c *Config) VerifyPassword(password string) bool {
	if c.AdminPasswordHash == "" {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(c.AdminPasswordHash), []byte(password)) == nil
}

// Load 从文件加载配置
// 向后兼容:
//   - 旧字段 admin_password (明文) 自动迁移为 admin_password_hash (bcrypt)
//   - 未配置密码时生成随机密码并打印到控制台
func Load(path string) (*Config, error) {
	cfg := Default()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// 配置文件不存在, 生成默认密码并保存
			// 不再硬编码默认设备, 需通过 Web 界面或 API 注册
			if err := initPassword(cfg); err != nil {
				return nil, err
			}
			if err := Save(path, cfg); err != nil {
				return nil, fmt.Errorf("保存默认配置失败: %w", err)
			}
			return cfg, nil
		}
		return nil, fmt.Errorf("读取配置失败: %w", err)
	}

	// 先解析为通用 map, 用于检测旧字段 admin_password
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("解析配置失败: %w", err)
	}

	// 解析到 Config 结构
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("解析配置失败: %w", err)
	}

	// 确保目录存在
	if cfg.DataDir == "" {
		cfg.DataDir = "./captures"
	}
	if cfg.DBPath == "" {
		cfg.DBPath = "./data/bio-recorder.db"
	}
	if err := os.MkdirAll(cfg.DataDir, 0755); err != nil {
		return nil, fmt.Errorf("创建数据目录失败: %w", err)
	}

	// 若配置了自定义保存目录, 确保该目录存在并使用它
	if cfg.CustomSaveDir != "" {
		if err := os.MkdirAll(cfg.CustomSaveDir, 0755); err != nil {
			return nil, fmt.Errorf("创建自定义保存目录失败: %w", err)
		}
		cfg.DataDir = cfg.CustomSaveDir
		slog.Info("使用自定义照片保存目录", "dir", cfg.CustomSaveDir)
	}

	// 密码迁移逻辑
	needSave := false
	if cfg.AdminPasswordHash == "" || !isBcryptHash(cfg.AdminPasswordHash) {
		// 检查旧字段 admin_password
		if rawPwd, ok := raw["admin_password"]; ok {
			var plaintext string
			if err := json.Unmarshal(rawPwd, &plaintext); err == nil && plaintext != "" {
				if isBcryptHash(plaintext) {
					// 已经是哈希, 直接采用
					cfg.AdminPasswordHash = plaintext
				} else {
					// 明文密码, 迁移为 bcrypt 哈希
					hash, err := HashPassword(plaintext)
					if err != nil {
						return nil, fmt.Errorf("密码哈希迁移失败: %w", err)
					}
					cfg.AdminPasswordHash = hash
					slog.Info("已将明文密码迁移为 bcrypt 哈希")
					needSave = true
				}
			}
		}

		// 仍未配置密码, 生成随机密码
		if cfg.AdminPasswordHash == "" || !isBcryptHash(cfg.AdminPasswordHash) {
			if err := initPassword(cfg); err != nil {
				return nil, err
			}
			needSave = true
		}
	}

	// 若旧配置未包含 retention_days 字段, 使用默认值 90
	if _, ok := raw["retention_days"]; !ok {
		cfg.RetentionDays = 90
	}

	// 回写迁移后的配置
	if needSave {
		if err := Save(path, cfg); err != nil {
			slog.Warn("回写迁移配置失败", "error", err)
		}
	}

	return cfg, nil
}

// initPassword 生成随机 16 字节密码, 打印到控制台, 哈希后存入配置
func initPassword(cfg *Config) error {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Errorf("生成随机密码失败: %w", err)
	}
	password := hex.EncodeToString(b) // 32 字符

	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	cfg.AdminPasswordHash = hash

	fmt.Println()
	fmt.Println("========================================================")
	fmt.Println("  已生成随机管理员密码 (请妥善保存):")
	fmt.Printf("    %s\n", password)
	fmt.Println("========================================================")
	fmt.Println()
	return nil
}

// Save 保存配置到文件
func Save(path string, cfg *Config) error {
	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("创建配置目录失败: %w", err)
		}
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}

	return os.WriteFile(path, data, 0600) // 仅所有者可读写
}
