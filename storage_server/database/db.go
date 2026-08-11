// ============================================================
//  【电脑端软件】运行目标: Windows / macOS / Linux / NAS 服务器
//  编译工具链: Go 1.21+ (跨平台单二进制, 不依赖 ESP32)
//  运行方式: 直接运行二进制 或 Docker 容器 / systemd 服务
// ============================================================
//
// Package database - SQLite 数据库管理
//
// 使用 modernc.org/sqlite (纯 Go 实现, 无 cgo 依赖, 确保跨平台编译)
// 提供 devices / photos / admin_sessions 表的 CRUD 操作
package database

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite" // 注册 sqlite 驱动
)

// DB 数据库管理器, 封装 *sql.DB
type DB struct {
	db *sql.DB
}

// Open 打开 (或创建) 数据库并完成初始化
// path 为数据库文件路径, 会自动创建父目录
func Open(path string) (*DB, error) {
	// 确保数据库文件所在目录存在
	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("创建数据库目录失败: %w", err)
		}
	}

	// modernc.org/sqlite 驱动名为 "sqlite"
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("打开数据库失败: %w", err)
	}

	// SQLite 单写入特性: 限制最大连接数为 1 可避免 "database is locked" 错误
	// 对本应用 (NAS / 单管理员) 的并发量足够
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("数据库连接失败: %w", err)
	}

	// 设置 SQLite 编译期/运行期参数
	pragmas := []string{
		"PRAGMA journal_mode=WAL;",      // WAL 模式, 提升并发读性能
		"PRAGMA busy_timeout=5000;",     // 写锁等待 5 秒
		"PRAGMA foreign_keys=ON;",       // 启用外键约束
		"PRAGMA synchronous=NORMAL;",    // WAL 下 NORMAL 足够安全且更快
		"PRAGMA temp_store=MEMORY;",     // 临时表存内存
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			db.Close()
			return nil, fmt.Errorf("执行 PRAGMA 失败 (%s): %w", p, err)
		}
	}

	d := &DB{db: db}

	// 执行表结构迁移
	if err := d.Migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("数据库迁移失败: %w", err)
	}

	slog.Info("数据库初始化完成", "path", path)
	return d, nil
}

// Close 关闭数据库连接
func (d *DB) Close() error {
	if d.db == nil {
		return nil
	}
	return d.db.Close()
}

// SQL 暴露底层 *sql.DB (供需要直接执行查询的场景使用)
func (d *DB) SQL() *sql.DB {
	return d.db
}
