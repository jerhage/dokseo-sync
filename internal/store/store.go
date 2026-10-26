package store

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"net/url"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

type Store struct {
	db *sql.DB
}

type Account struct {
	ID       int64
	Username string
}

func Open(location string) (*Store, error) {
	query := url.Values{}
	query.Add("_pragma", "journal_mode(WAL)")
	query.Add("_pragma", "busy_timeout(5000)")
	query.Add("_pragma", "foreign_keys(ON)")
	db, err := sql.Open("sqlite", dataSourceName(location, query))
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	if isMemory(location) {
		db.SetMaxOpenConns(1)
	}
	store := &Store{db: db}
	if err := store.migrate(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func isMemory(location string) bool {
	return location == ":memory:" || strings.Contains(location, "mode=memory")
}

func dataSourceName(location string, pragmas url.Values) string {
	if !strings.HasPrefix(location, "file:") {
		location = "file:" + location
	}
	separator := "?"
	if strings.Contains(location, "?") {
		separator = "&"
	}
	return location + separator + pragmas.Encode()
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version TEXT PRIMARY KEY,
		applied_at INTEGER NOT NULL
	)`)
	if err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}
	names, err := fs.Glob(migrationFiles, "migrations/*.sql")
	if err != nil {
		return err
	}
	sort.Strings(names)
	for _, name := range names {
		if err := s.applyMigration(ctx, name); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) applyMigration(ctx context.Context, name string) error {
	version := strings.TrimSuffix(strings.TrimPrefix(name, "migrations/"), ".sql")
	var applied int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, version).Scan(&applied)
	if err != nil {
		return fmt.Errorf("check migration %s: %w", version, err)
	}
	if applied > 0 {
		return nil
	}
	script, err := migrationFiles.ReadFile(name)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, string(script)); err != nil {
		return fmt.Errorf("apply migration %s: %w", version, err)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)`,
		version, time.Now().UnixMilli())
	if err != nil {
		return fmt.Errorf("record migration %s: %w", version, err)
	}
	return tx.Commit()
}

func (s *Store) EnsureAccount(ctx context.Context, username string) (Account, error) {
	_, err := s.db.ExecContext(ctx, `INSERT INTO accounts (username) VALUES (?) ON CONFLICT(username) DO NOTHING`, username)
	if err != nil {
		return Account{}, fmt.Errorf("ensure account: %w", err)
	}
	account, found, err := s.AccountByUsername(ctx, username)
	if err != nil {
		return Account{}, err
	}
	if !found {
		return Account{}, fmt.Errorf("account %q missing after insert", username)
	}
	return account, nil
}

func (s *Store) AccountByUsername(ctx context.Context, username string) (Account, bool, error) {
	var account Account
	err := s.db.QueryRowContext(ctx, `SELECT id, username FROM accounts WHERE username = ?`, username).
		Scan(&account.ID, &account.Username)
	if err == sql.ErrNoRows {
		return Account{}, false, nil
	}
	if err != nil {
		return Account{}, false, fmt.Errorf("find account: %w", err)
	}
	return account, true, nil
}
