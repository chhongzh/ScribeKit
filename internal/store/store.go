// Package store 在 ent 之上提供业务层读写，返回的结构体直接对应 REST/WS 的 JSON 形状。
package store

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"

	"github.com/chhongzh/scribekit/internal/store/ent"
	"github.com/chhongzh/scribekit/internal/store/ent/suggestion"
)

type Store struct {
	client *ent.Client
}

// Open 在 dataDir 下打开（必要时创建）scribekit.db 并迁移表结构。
func Open(ctx context.Context, dataDir string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, err
	}
	return OpenDSN(ctx, dsnFor(filepath.Join(dataDir, "scribekit.db")))
}

func dsnFor(file string) string {
	q := url.Values{}
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "journal_mode(WAL)")
	q.Set("_time_format", "sqlite")
	return "file:" + filepath.ToSlash(file) + "?" + q.Encode()
}

// OpenDSN 供测试传入内存库。
func OpenDSN(ctx context.Context, dsn string) (*Store, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	client := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.SQLite, db)))
	if err := client.Schema.Create(ctx); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	s := &Store{client: client}
	// 上次进程异常退出时遗留的流式半成品没有意义。
	if _, err := client.Suggestion.Delete().Where(suggestion.StatusEQ("streaming")).Exec(ctx); err != nil {
		_ = client.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.client.Close() }

func (s *Store) tx(ctx context.Context, fn func(tx *ent.Tx) error) error {
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		if rerr := tx.Rollback(); rerr != nil {
			return fmt.Errorf("%w (rollback: %v)", err, rerr)
		}
		return err
	}
	return tx.Commit()
}

func notFoundErr(err error, what string, id int) error {
	if ent.IsNotFound(err) {
		return notFound(what, id)
	}
	return err
}
