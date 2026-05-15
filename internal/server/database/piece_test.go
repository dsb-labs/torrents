package database_test

import (
	"path/filepath"
	"testing"

	"github.com/anacrolix/torrent/metainfo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dsb-labs/torrents/internal/server/database"
)

func TestPieceRepository_Get(t *testing.T) {
	t.Parallel()

	t.Run("unknown returns ok-false", func(t *testing.T) {
		repo := newTestPieceRepository(t)

		got, err := repo.Get(testPieceKey(t, "1111111111111111111111111111111111111111", 5))
		require.NoError(t, err)
		assert.False(t, got.Ok)
		assert.False(t, got.Complete)
	})

	t.Run("known complete", func(t *testing.T) {
		repo := newTestPieceRepository(t)
		key := testPieceKey(t, "2222222222222222222222222222222222222222", 7)

		require.NoError(t, repo.Set(key, true))

		got, err := repo.Get(key)
		require.NoError(t, err)
		assert.True(t, got.Ok)
		assert.True(t, got.Complete)
	})

	t.Run("known not complete", func(t *testing.T) {
		repo := newTestPieceRepository(t)
		key := testPieceKey(t, "3333333333333333333333333333333333333333", 0)

		require.NoError(t, repo.Set(key, false))

		got, err := repo.Get(key)
		require.NoError(t, err)
		assert.True(t, got.Ok)
		assert.False(t, got.Complete)
	})
}

func TestPieceRepository_Set(t *testing.T) {
	t.Parallel()

	t.Run("overwrites existing row", func(t *testing.T) {
		repo := newTestPieceRepository(t)
		key := testPieceKey(t, "4444444444444444444444444444444444444444", 12)

		require.NoError(t, repo.Set(key, true))
		require.NoError(t, repo.Set(key, false))

		got, err := repo.Get(key)
		require.NoError(t, err)
		assert.True(t, got.Ok)
		assert.False(t, got.Complete)
	})

	t.Run("keys with same hash different index are independent", func(t *testing.T) {
		repo := newTestPieceRepository(t)
		hash := "5555555555555555555555555555555555555555"

		require.NoError(t, repo.Set(testPieceKey(t, hash, 0), true))
		require.NoError(t, repo.Set(testPieceKey(t, hash, 1), false))

		got, err := repo.Get(testPieceKey(t, hash, 0))
		require.NoError(t, err)
		assert.True(t, got.Complete)

		got, err = repo.Get(testPieceKey(t, hash, 1))
		require.NoError(t, err)
		assert.False(t, got.Complete)
	})
}

func TestPieceRepository_Close(t *testing.T) {
	t.Parallel()

	repo := newTestPieceRepository(t)

	require.NoError(t, repo.Close())
	require.NoError(t, repo.Close(), "Close must be safe to call repeatedly")

	got, err := repo.Get(testPieceKey(t, "6666666666666666666666666666666666666666", 0))
	require.NoError(t, err, "Close must not affect subsequent operations")
	assert.False(t, got.Ok)
}

func TestPieceRepository_Persistent(t *testing.T) {
	t.Parallel()

	repo := newTestPieceRepository(t)
	assert.True(t, repo.Persistent())
}

func TestPieceRepository_CascadesFromTorrentDelete(t *testing.T) {
	t.Parallel()

	db, err := database.Open(t.Context(), database.Config{
		Logger: newTestLogger(t),
		Path:   filepath.Join(t.TempDir(), "test.db"),
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	torrents := database.NewTorrentRepository(db)
	pieces := database.NewPieceRepository(db)

	ctx := t.Context()
	hash := "7777777777777777777777777777777777777777"

	require.NoError(t, torrents.Create(ctx, database.Torrent{
		InfoHash:  hash,
		Magnet:    "magnet:?xt=urn:btih:" + hash,
		TargetDir: "/tmp/downloads",
	}))

	require.NoError(t, pieces.Set(testPieceKey(t, hash, 0), true))
	require.NoError(t, pieces.Set(testPieceKey(t, hash, 1), false))

	require.NoError(t, torrents.Delete(ctx, hash))

	var rowCount int
	err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM piece_completion WHERE info_hash = ?`, hash).Scan(&rowCount)
	require.NoError(t, err)
	assert.Equal(t, 0, rowCount, "piece_completion rows should be cascaded when their torrent is deleted")
}

func TestPieceRepository_Load(t *testing.T) {
	t.Parallel()

	db, err := database.Open(t.Context(), database.Config{
		Logger: newTestLogger(t),
		Path:   filepath.Join(t.TempDir(), "test.db"),
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	seeder := database.NewPieceRepository(db)
	hash := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	require.NoError(t, seeder.Set(testPieceKey(t, hash, 0), true))
	require.NoError(t, seeder.Set(testPieceKey(t, hash, 1), false))

	repo := database.NewPieceRepository(db)

	got, err := repo.Get(testPieceKey(t, hash, 0))
	require.NoError(t, err)
	assert.False(t, got.Ok, "cache should be empty before Load")

	require.NoError(t, repo.Load(t.Context()))

	got, err = repo.Get(testPieceKey(t, hash, 0))
	require.NoError(t, err)
	assert.True(t, got.Ok)
	assert.True(t, got.Complete)

	got, err = repo.Get(testPieceKey(t, hash, 1))
	require.NoError(t, err)
	assert.True(t, got.Ok)
	assert.False(t, got.Complete)
}

func TestPieceRepository_Forget(t *testing.T) {
	t.Parallel()

	repo := newTestPieceRepository(t)
	target := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	other := "cccccccccccccccccccccccccccccccccccccccc"

	require.NoError(t, repo.Set(testPieceKey(t, target, 0), true))
	require.NoError(t, repo.Set(testPieceKey(t, target, 1), false))
	require.NoError(t, repo.Set(testPieceKey(t, other, 0), true))

	repo.Forget(target)

	for _, idx := range []int{0, 1} {
		got, err := repo.Get(testPieceKey(t, target, idx))
		require.NoError(t, err)
		assert.False(t, got.Ok, "forgotten piece %d should report unknown", idx)
	}

	got, err := repo.Get(testPieceKey(t, other, 0))
	require.NoError(t, err)
	assert.True(t, got.Ok, "Forget must not touch other torrents")
	assert.True(t, got.Complete)
}

func newTestPieceRepository(t *testing.T) *database.PieceRepository {
	t.Helper()

	db, err := database.Open(t.Context(), database.Config{
		Logger: newTestLogger(t),
		Path:   filepath.Join(t.TempDir(), "test.db"),
	})
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})

	return database.NewPieceRepository(db)
}

func testPieceKey(t *testing.T, hexHash string, index int) metainfo.PieceKey {
	t.Helper()

	var h metainfo.Hash
	require.NoError(t, h.FromHexString(hexHash))

	return metainfo.PieceKey{InfoHash: h, Index: index}
}
