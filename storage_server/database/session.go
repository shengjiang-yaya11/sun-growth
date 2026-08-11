// ============================================================
//  【电脑端软件】运行目标: Windows / macOS / Linux / NAS 服务器
//  编译工具链: Go 1.21+ (跨平台单二进制, 不依赖 ESP32)
//  运行方式: 直接运行二进制 或 Docker 容器 / systemd 服务
// ============================================================
//
// Package database - 管理员会话管理 (替代内存 map)
//
// 会话 token 使用 crypto/rand 生成, 存储到 SQLite admin_sessions 表
package database

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log/slog"
	"time"
)

// sessionTTL 会话有效期
const sessionTTL = 24 * time.Hour

// SessionStore 基于 SQLite 的会话存储, 替代内存 map
type SessionStore struct {
	db        *DB
	tlsEnabled bool // TLS 启用时 cookie 设置 Secure 标志
}

// NewSessionStore 创建会话存储
func NewSessionStore(db *DB, tlsEnabled bool) *SessionStore {
	return &SessionStore{db: db, tlsEnabled: tlsEnabled}
}

// TLSEnabled 返回是否启用 TLS (供 cookie Secure 标志使用)
func (s *SessionStore) TLSEnabled() bool {
	return s.tlsEnabled
}

// Create 生成新会话 token 并写入数据库, 返回 token
func (s *SessionStore) Create(ip string) (string, error) {
	// 使用 crypto/rand 生成 32 字节随机 token
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("生成会话 token 失败: %w", err)
	}
	token := hex.EncodeToString(b)

	expiresAt := time.Now().Add(sessionTTL)
	if _, err := s.db.db.Exec(
		`INSERT INTO admin_sessions (session_token, expires_at, ip_address) VALUES (?, ?, ?)`,
		token, expiresAt.UTC(), ip,
	); err != nil {
		return "", fmt.Errorf("写入会话失败: %w", err)
	}
	return token, nil
}

// Valid 校验 token 是否有效 (存在且未过期)
func (s *SessionStore) Valid(token string) bool {
	if token == "" {
		return false
	}
	var expiresAt string
	err := s.db.db.QueryRow(
		`SELECT expires_at FROM admin_sessions WHERE session_token = ?`,
		token,
	).Scan(&expiresAt)
	if err != nil {
		if err != sql.ErrNoRows {
			slog.Warn("查询会话失败", "error", err)
		}
		return false
	}
	t := parseTime(expiresAt)
	if t.IsZero() {
		return false
	}
	// 已过期则清除
	if time.Now().After(t) {
		_ = s.Destroy(token)
		return false
	}
	return true
}

// Destroy 删除指定会话
func (s *SessionStore) Destroy(token string) error {
	if token == "" {
		return nil
	}
	_, err := s.db.db.Exec(`DELETE FROM admin_sessions WHERE session_token = ?`, token)
	if err != nil {
		return fmt.Errorf("删除会话失败: %w", err)
	}
	return nil
}

// CleanExpiredSessions 清理所有过期会话, 返回清理数量
func (s *SessionStore) CleanExpiredSessions() (int64, error) {
	res, err := s.db.db.Exec(
		`DELETE FROM admin_sessions WHERE expires_at < ?`,
		time.Now().UTC(),
	)
	if err != nil {
		return 0, fmt.Errorf("清理过期会话失败: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("获取清理数量失败: %w", err)
	}
	return n, nil
}
