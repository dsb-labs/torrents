// Package database provides the SQLite-backed persistence layer for the torrents server.
package database

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/golang-migrate/migrate/v4"
	sqlitemigrate "github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"modernc.org/sqlite"
)

const (
	sqliteConstraintPrimaryKey = 1555
	sqliteConstraintUnique     = 2067
)

//go:embed migrations/*.sql
var migrations embed.FS

// The Config type contains fields used to open the database.
type Config struct {
	// The logger used for database lifecycle events.
	Logger *slog.Logger
	// The filesystem path to the SQLite database file.
	Path string
}

// Open opens (or creates) the SQLite database at the path in config and runs any
// pending migrations. The returned pool must be closed by the caller when no
// longer needed.
//
// The connection is opened in WAL journal mode with a 5s busy timeout. WAL
// lets readers run concurrently with a single writer, and the timeout makes
// SQLite wait for a contended lock instead of immediately returning SQLITE_BUSY
// — both are necessary because the torrent client hammers piece_completion
// writes from many goroutines while the app reads/writes the torrent table.
func Open(ctx context.Context, config Config) (*sql.DB, error) {
	logger := config.Logger.With("component", "database")

	db, err := sql.Open("sqlite", config.Path+"?_pragma=journal_mode(wal)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err = db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	if err = migrateUp(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	logger.With("path", config.Path).Debug("database opened")

	return db, nil
}

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func parseTimestamps(createdAt, updatedAt string) (time.Time, time.Time, error) {
	created, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("failed to parse created_at: %w", err)
	}

	updated, err := time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("failed to parse updated_at: %w", err)
	}

	return created, updated, nil
}

// IsUniqueError reports whether err is a unique-key or primary-key constraint violation.
func IsUniqueError(err error) bool {
	var sqliteErr *sqlite.Error
	if !errors.As(err, &sqliteErr) {
		return false
	}

	return sqliteErr.Code() == sqliteConstraintPrimaryKey || sqliteErr.Code() == sqliteConstraintUnique
}

func migrateUp(db *sql.DB) error {
	m, err := newMigrator(db)
	if err != nil {
		return err
	}

	if err = m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("failed to apply migrations: %w", err)
	}

	return nil
}

func newMigrator(db *sql.DB) (*migrate.Migrate, error) {
	src, err := iofs.New(migrations, "migrations")
	if err != nil {
		return nil, fmt.Errorf("failed to load migration source: %w", err)
	}

	driver, err := sqlitemigrate.WithInstance(db, &sqlitemigrate.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to construct migration driver: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", src, "sqlite", driver)
	if err != nil {
		return nil, fmt.Errorf("failed to construct migrator: %w", err)
	}

	return m, nil
}
