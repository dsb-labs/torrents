package database_test

import (
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dsb-labs/torrents/internal/server/database"
)

func TestTorrentRepository_Create(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		repo := newTestRepository(t)
		ctx := t.Context()

		before := time.Now().UTC().Add(-time.Second)

		err := repo.Create(ctx, database.Torrent{
			InfoHash:  "0123456789abcdef0123456789abcdef01234567",
			Magnet:    "magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567",
			Label:     "test torrent",
			TargetDir: "/tmp/downloads",
		})
		require.NoError(t, err)

		got, err := repo.Get(ctx, "0123456789abcdef0123456789abcdef01234567")
		require.NoError(t, err)

		assert.Equal(t, "0123456789abcdef0123456789abcdef01234567", got.InfoHash)
		assert.Equal(t, "magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567", got.Magnet)
		assert.Equal(t, "test torrent", got.Label)
		assert.Equal(t, "/tmp/downloads", got.TargetDir)
		assert.False(t, got.Paused)
		assert.WithinDuration(t, time.Now().UTC(), got.CreatedAt, 5*time.Second)
		assert.False(t, got.CreatedAt.Before(before))
		assert.Equal(t, got.CreatedAt, got.UpdatedAt)
	})

	t.Run("ignores input-controlled fields", func(t *testing.T) {
		repo := newTestRepository(t)
		ctx := t.Context()

		bogus := time.Date(1999, 1, 1, 0, 0, 0, 0, time.UTC)

		require.NoError(t, repo.Create(ctx, database.Torrent{
			InfoHash:  "dddddddddddddddddddddddddddddddddddddddd",
			Magnet:    "magnet:?xt=urn:btih:dddddddddddddddddddddddddddddddddddddddd",
			TargetDir: "/tmp/downloads",
			Paused:  true,
			CreatedAt: bogus,
			UpdatedAt: bogus,
		}))

		got, err := repo.Get(ctx, "dddddddddddddddddddddddddddddddddddddddd")
		require.NoError(t, err)

		assert.False(t, got.Paused, "Create must default Paused to false")
		assert.WithinDuration(t, time.Now().UTC(), got.CreatedAt, 5*time.Second, "Create must assign CreatedAt")
		assert.WithinDuration(t, time.Now().UTC(), got.UpdatedAt, 5*time.Second, "Create must assign UpdatedAt")
	})

	t.Run("duplicate", func(t *testing.T) {
		repo := newTestRepository(t)
		ctx := t.Context()

		torrent := database.Torrent{
			InfoHash:  "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			Magnet:    "magnet:?xt=urn:btih:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			TargetDir: "/tmp/downloads",
		}

		require.NoError(t, repo.Create(ctx, torrent))
		err := repo.Create(ctx, torrent)
		assert.ErrorIs(t, err, database.ErrTorrentAlreadyExists)
	})
}

func TestTorrentRepository_Get(t *testing.T) {
	t.Parallel()

	t.Run("missing", func(t *testing.T) {
		repo := newTestRepository(t)

		_, err := repo.Get(t.Context(), "ffffffffffffffffffffffffffffffffffffffff")
		assert.ErrorIs(t, err, database.ErrTorrentNotFound)
	})
}

func TestTorrentRepository_List(t *testing.T) {
	t.Parallel()

	t.Run("empty", func(t *testing.T) {
		repo := newTestRepository(t)

		torrents, err := repo.List(t.Context())
		require.NoError(t, err)
		assert.Empty(t, torrents)
	})

	t.Run("multiple", func(t *testing.T) {
		repo := newTestRepository(t)
		ctx := t.Context()

		hashes := []string{
			"1111111111111111111111111111111111111111",
			"2222222222222222222222222222222222222222",
			"3333333333333333333333333333333333333333",
		}

		for _, hash := range hashes {
			require.NoError(t, repo.Create(ctx, database.Torrent{
				InfoHash:  hash,
				Magnet:    "magnet:?xt=urn:btih:" + hash,
				TargetDir: "/tmp/downloads",
			}))
		}

		got, err := repo.List(ctx)
		require.NoError(t, err)
		require.Len(t, got, 3)

		gotHashes := make([]string, len(got))
		for i, torrent := range got {
			gotHashes[i] = torrent.InfoHash
		}
		assert.ElementsMatch(t, hashes, gotHashes)
	})
}

func TestTorrentRepository_Delete(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		repo := newTestRepository(t)
		ctx := t.Context()

		require.NoError(t, repo.Create(ctx, database.Torrent{
			InfoHash:  "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			Magnet:    "magnet:?xt=urn:btih:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			TargetDir: "/tmp/downloads",
		}))

		require.NoError(t, repo.Delete(ctx, "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"))

		_, err := repo.Get(ctx, "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
		assert.ErrorIs(t, err, database.ErrTorrentNotFound)
	})

	t.Run("missing", func(t *testing.T) {
		repo := newTestRepository(t)

		err := repo.Delete(t.Context(), "ffffffffffffffffffffffffffffffffffffffff")
		assert.ErrorIs(t, err, database.ErrTorrentNotFound)
	})
}

func TestTorrentRepository_SetPaused(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		repo := newTestRepository(t)
		ctx := t.Context()

		hash := "cccccccccccccccccccccccccccccccccccccccc"
		require.NoError(t, repo.Create(ctx, database.Torrent{
			InfoHash:  hash,
			Magnet:    "magnet:?xt=urn:btih:" + hash,
			TargetDir: "/tmp/downloads",
		}))

		require.NoError(t, repo.SetPaused(ctx, hash, true))

		got, err := repo.Get(ctx, hash)
		require.NoError(t, err)
		assert.True(t, got.Paused)

		require.NoError(t, repo.SetPaused(ctx, hash, false))

		got, err = repo.Get(ctx, hash)
		require.NoError(t, err)
		assert.False(t, got.Paused)
	})

	t.Run("missing", func(t *testing.T) {
		repo := newTestRepository(t)

		err := repo.SetPaused(t.Context(), "ffffffffffffffffffffffffffffffffffffffff", true)
		assert.ErrorIs(t, err, database.ErrTorrentNotFound)
	})
}

func newTestRepository(t *testing.T) *database.TorrentRepository {
	t.Helper()

	db, err := database.Open(t.Context(), database.Config{
		Logger: newTestLogger(t),
		Path:   filepath.Join(t.TempDir(), "test.db"),
	})
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})

	return database.NewTorrentRepository(db)
}

func newTestLogger(t *testing.T) *slog.Logger {
	t.Helper()

	level := slog.LevelError
	if testing.Verbose() {
		level = slog.LevelDebug
	}

	return slog.New(slog.NewTextHandler(t.Output(), &slog.HandlerOptions{
		AddSource: testing.Verbose(),
		Level:     level,
	}))
}
