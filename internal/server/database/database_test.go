package database

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	"github.com/stretchr/testify/require"

	_ "modernc.org/sqlite"
)

func TestMigrations(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "test.db")

	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	require.NoError(t, db.PingContext(t.Context()))

	t.Run("up", func(t *testing.T) {
		require.NoError(t, migrateUp(db))
	})

	t.Run("down", func(t *testing.T) {
		m, err := newMigrator(db)
		require.NoError(t, err)

		err = m.Down()
		if err != nil && !errors.Is(err, migrate.ErrNoChange) {
			require.NoError(t, err)
		}
	})

	t.Run("up again", func(t *testing.T) {
		require.NoError(t, migrateUp(db))
	})
}
