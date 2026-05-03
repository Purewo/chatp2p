package storage

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type Config struct {
	Driver string
	DSN    string
}

func Open(cfg Config) (*sql.DB, error) {
	if cfg.Driver == "" {
		cfg.Driver = "sqlite"
	}
	if cfg.DSN == "" {
		cfg.DSN = "file:chatp2p.db"
	}

	db, err := sql.Open(cfg.Driver, cfg.DSN)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}

	if strings.Contains(strings.ToLower(cfg.Driver), "sqlite") {
		if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
			_ = db.Close()
			return nil, err
		}
	}

	return db, nil
}

func Migrate(ctx context.Context, db *sql.DB, dir string) error {
	if err := ensureMigrationsTable(ctx, db); err != nil {
		return err
	}

	files, err := filepath.Glob(filepath.Join(dir, "*.sql"))
	if err != nil {
		return err
	}
	sort.Strings(files)

	for _, file := range files {
		version := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
		applied, err := migrationApplied(ctx, db, version)
		if err != nil {
			return err
		}
		if applied {
			continue
		}

		sqlBytes, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", file, err)
		}

		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}

		if _, err := tx.ExecContext(ctx, string(sqlBytes)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply migration %s: %w", version, err)
		}

		if _, err := tx.ExecContext(ctx, `
			INSERT INTO schema_migrations (version, applied_at)
			VALUES (?, ?)
		`, version, time.Now().UTC().Unix()); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record migration %s: %w", version, err)
		}

		if err := tx.Commit(); err != nil {
			return err
		}
	}

	return nil
}

func ensureMigrationsTable(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at INTEGER NOT NULL
		)
	`)
	return err
}

func migrationApplied(ctx context.Context, db *sql.DB, version string) (bool, error) {
	row := db.QueryRowContext(ctx, `
		SELECT 1
		FROM schema_migrations
		WHERE version = ?
	`, version)

	var one int
	switch err := row.Scan(&one); {
	case err == nil:
		return true, nil
	case err == sql.ErrNoRows:
		return false, nil
	default:
		return false, err
	}
}
