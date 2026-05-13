package service_test

import (
	"bytes"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/dsb-labs/torrents/internal/server/database"
	"github.com/dsb-labs/torrents/internal/server/service"
	"github.com/dsb-labs/torrents/internal/server/torrent"
)

const testInfoHash = "0123456789abcdef0123456789abcdef01234567"

func TestTorrentService_AddMagnet(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		engine, repo := newServiceFixture(t)
		svc := service.NewTorrentService(newTestLogger(t), engine, repo)

		engine.EXPECT().AddMagnet(mock.Anything, "magnet:?xt=urn:btih:"+testInfoHash).Return(testInfoHash, nil).Once()
		repo.EXPECT().Create(mock.Anything, database.Torrent{
			InfoHash:  testInfoHash,
			Magnet:    "magnet:?xt=urn:btih:" + testInfoHash,
			Label:     "linux iso",
			TargetDir: "/data/downloads",
		}).Return(nil).Once()
		repo.EXPECT().Get(mock.Anything, testInfoHash).Return(database.Torrent{
			InfoHash:  testInfoHash,
			Magnet:    "magnet:?xt=urn:btih:" + testInfoHash,
			Label:     "linux iso",
			TargetDir: "/data/downloads",
			CreatedAt: time.Unix(1700000000, 0).UTC(),
			UpdatedAt: time.Unix(1700000000, 0).UTC(),
		}, nil).Once()
		engine.EXPECT().Snapshot(torrent.InfoHash(testInfoHash)).Return(torrent.Progress{
			InfoHash:       testInfoHash,
			Name:           "linux-amd64.iso",
			Length:         500_000_000,
			BytesCompleted: 0,
			ActivePeers:    2,
			Seeders:        1,
		}, nil).Once()

		got, err := svc.AddMagnet(t.Context(), "magnet:?xt=urn:btih:"+testInfoHash, service.AddOptions{
			Label:     "linux iso",
			TargetDir: "/data/downloads",
		})
		require.NoError(t, err)

		assert.Equal(t, testInfoHash, got.InfoHash)
		assert.Equal(t, "linux iso", got.Label)
		assert.Equal(t, "linux-amd64.iso", got.Name)
		assert.EqualValues(t, 500_000_000, got.Length)
		assert.Equal(t, 2, got.ActivePeers)
	})

	t.Run("duplicate", func(t *testing.T) {
		engine, repo := newServiceFixture(t)
		svc := service.NewTorrentService(newTestLogger(t), engine, repo)

		engine.EXPECT().AddMagnet(mock.Anything, mock.Anything).Return(testInfoHash, nil).Once()
		repo.EXPECT().Create(mock.Anything, mock.Anything).Return(database.ErrTorrentAlreadyExists).Once()

		_, err := svc.AddMagnet(t.Context(), "magnet:?xt=urn:btih:"+testInfoHash, service.AddOptions{})
		assert.ErrorIs(t, err, service.ErrTorrentAlreadyExists)
	})

	t.Run("engine error", func(t *testing.T) {
		engine, repo := newServiceFixture(t)
		svc := service.NewTorrentService(newTestLogger(t), engine, repo)

		engine.EXPECT().AddMagnet(mock.Anything, mock.Anything).Return("", errors.New("invalid uri")).Once()

		_, err := svc.AddMagnet(t.Context(), "bad", service.AddOptions{})
		assert.ErrorContains(t, err, "failed to add magnet")
	})
}

func TestTorrentService_AddFile(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		engine, repo := newServiceFixture(t)
		svc := service.NewTorrentService(newTestLogger(t), engine, repo)

		engine.EXPECT().AddFile(mock.Anything, mock.Anything).Return(testInfoHash, nil).Once()
		repo.EXPECT().Create(mock.Anything, mock.MatchedBy(func(t database.Torrent) bool {
			return t.InfoHash == testInfoHash && t.Magnet == "magnet:?xt=urn:btih:"+testInfoHash
		})).Return(nil).Once()
		repo.EXPECT().Get(mock.Anything, testInfoHash).Return(database.Torrent{InfoHash: testInfoHash}, nil).Once()
		engine.EXPECT().Snapshot(torrent.InfoHash(testInfoHash)).Return(torrent.Progress{}, torrent.ErrTorrentNotFound).Once()

		got, err := svc.AddFile(t.Context(), bytes.NewReader([]byte("fake-torrent")), service.AddOptions{})
		require.NoError(t, err)
		assert.Equal(t, testInfoHash, got.InfoHash)
	})
}

func TestTorrentService_Get(t *testing.T) {
	t.Parallel()

	t.Run("success with live state", func(t *testing.T) {
		engine, repo := newServiceFixture(t)
		svc := service.NewTorrentService(newTestLogger(t), engine, repo)

		row := database.Torrent{
			InfoHash:  testInfoHash,
			Magnet:    "magnet:?xt=urn:btih:" + testInfoHash,
			Label:     "iso",
			TargetDir: "/data",
			IsPaused:  false,
			CreatedAt: time.Unix(1700000000, 0).UTC(),
			UpdatedAt: time.Unix(1700000000, 0).UTC(),
		}
		repo.EXPECT().Get(mock.Anything, testInfoHash).Return(row, nil).Once()
		engine.EXPECT().Snapshot(torrent.InfoHash(testInfoHash)).Return(torrent.Progress{
			Name:           "thing.iso",
			Length:         1024,
			BytesCompleted: 256,
			ActivePeers:    3,
			Seeders:        1,
		}, nil).Once()

		got, err := svc.Get(t.Context(), testInfoHash)
		require.NoError(t, err)
		assert.Equal(t, "thing.iso", got.Name)
		assert.EqualValues(t, 1024, got.Length)
		assert.EqualValues(t, 256, got.BytesCompleted)
	})

	t.Run("success engine missing", func(t *testing.T) {
		engine, repo := newServiceFixture(t)
		svc := service.NewTorrentService(newTestLogger(t), engine, repo)

		repo.EXPECT().Get(mock.Anything, testInfoHash).Return(database.Torrent{InfoHash: testInfoHash}, nil).Once()
		engine.EXPECT().Snapshot(torrent.InfoHash(testInfoHash)).Return(torrent.Progress{}, torrent.ErrTorrentNotFound).Once()

		got, err := svc.Get(t.Context(), testInfoHash)
		require.NoError(t, err)
		assert.Equal(t, testInfoHash, got.InfoHash)
		assert.Empty(t, got.Name)
	})

	t.Run("not found", func(t *testing.T) {
		engine, repo := newServiceFixture(t)
		svc := service.NewTorrentService(newTestLogger(t), engine, repo)

		repo.EXPECT().Get(mock.Anything, testInfoHash).Return(database.Torrent{}, database.ErrTorrentNotFound).Once()

		_, err := svc.Get(t.Context(), testInfoHash)
		assert.ErrorIs(t, err, service.ErrTorrentNotFound)
	})
}

func TestTorrentService_List(t *testing.T) {
	t.Parallel()

	t.Run("empty", func(t *testing.T) {
		engine, repo := newServiceFixture(t)
		svc := service.NewTorrentService(newTestLogger(t), engine, repo)

		_ = engine
		repo.EXPECT().List(mock.Anything).Return(nil, nil).Once()

		got, err := svc.List(t.Context())
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("multiple", func(t *testing.T) {
		engine, repo := newServiceFixture(t)
		svc := service.NewTorrentService(newTestLogger(t), engine, repo)

		hashA := "1111111111111111111111111111111111111111"
		hashB := "2222222222222222222222222222222222222222"

		repo.EXPECT().List(mock.Anything).Return([]database.Torrent{
			{InfoHash: hashA},
			{InfoHash: hashB},
		}, nil).Once()
		engine.EXPECT().Snapshot(torrent.InfoHash(hashA)).Return(torrent.Progress{Name: "A", BytesCompleted: 10}, nil).Once()
		engine.EXPECT().Snapshot(torrent.InfoHash(hashB)).Return(torrent.Progress{}, torrent.ErrTorrentNotFound).Once()

		got, err := svc.List(t.Context())
		require.NoError(t, err)
		require.Len(t, got, 2)
		assert.Equal(t, "A", got[0].Name)
		assert.EqualValues(t, 10, got[0].BytesCompleted)
		assert.Empty(t, got[1].Name)
	})
}

func TestTorrentService_Remove(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		engine, repo := newServiceFixture(t)
		svc := service.NewTorrentService(newTestLogger(t), engine, repo)

		engine.EXPECT().Remove(mock.Anything, torrent.InfoHash(testInfoHash)).Return(nil).Once()
		repo.EXPECT().Delete(mock.Anything, testInfoHash).Return(nil).Once()

		require.NoError(t, svc.Remove(t.Context(), testInfoHash))
	})

	t.Run("not found in engine still deletes row", func(t *testing.T) {
		engine, repo := newServiceFixture(t)
		svc := service.NewTorrentService(newTestLogger(t), engine, repo)

		engine.EXPECT().Remove(mock.Anything, torrent.InfoHash(testInfoHash)).Return(torrent.ErrTorrentNotFound).Once()
		repo.EXPECT().Delete(mock.Anything, testInfoHash).Return(nil).Once()

		require.NoError(t, svc.Remove(t.Context(), testInfoHash))
	})

	t.Run("not found", func(t *testing.T) {
		engine, repo := newServiceFixture(t)
		svc := service.NewTorrentService(newTestLogger(t), engine, repo)

		engine.EXPECT().Remove(mock.Anything, torrent.InfoHash(testInfoHash)).Return(torrent.ErrTorrentNotFound).Once()
		repo.EXPECT().Delete(mock.Anything, testInfoHash).Return(database.ErrTorrentNotFound).Once()

		err := svc.Remove(t.Context(), testInfoHash)
		assert.ErrorIs(t, err, service.ErrTorrentNotFound)
	})
}

func TestTorrentService_Pause(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		engine, repo := newServiceFixture(t)
		svc := service.NewTorrentService(newTestLogger(t), engine, repo)

		engine.EXPECT().Pause(mock.Anything, torrent.InfoHash(testInfoHash)).Return(nil).Once()
		repo.EXPECT().SetPaused(mock.Anything, testInfoHash, true).Return(nil).Once()

		require.NoError(t, svc.Pause(t.Context(), testInfoHash))
	})

	t.Run("not found in engine", func(t *testing.T) {
		engine, repo := newServiceFixture(t)
		svc := service.NewTorrentService(newTestLogger(t), engine, repo)

		engine.EXPECT().Pause(mock.Anything, torrent.InfoHash(testInfoHash)).Return(torrent.ErrTorrentNotFound).Once()

		err := svc.Pause(t.Context(), testInfoHash)
		assert.ErrorIs(t, err, service.ErrTorrentNotFound)
	})
}

func TestTorrentService_Resume(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		engine, repo := newServiceFixture(t)
		svc := service.NewTorrentService(newTestLogger(t), engine, repo)

		engine.EXPECT().Resume(mock.Anything, torrent.InfoHash(testInfoHash)).Return(nil).Once()
		repo.EXPECT().SetPaused(mock.Anything, testInfoHash, false).Return(nil).Once()

		require.NoError(t, svc.Resume(t.Context(), testInfoHash))
	})
}

func TestTorrentService_Restore(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		engine, repo := newServiceFixture(t)
		svc := service.NewTorrentService(newTestLogger(t), engine, repo)

		hashA := "1111111111111111111111111111111111111111"
		hashB := "2222222222222222222222222222222222222222"

		repo.EXPECT().List(mock.Anything).Return([]database.Torrent{
			{InfoHash: hashA, Magnet: "magnet:?xt=urn:btih:" + hashA, IsPaused: false},
			{InfoHash: hashB, Magnet: "magnet:?xt=urn:btih:" + hashB, IsPaused: true},
		}, nil).Once()
		engine.EXPECT().AddMagnet(mock.Anything, "magnet:?xt=urn:btih:"+hashA).Return(torrent.InfoHash(hashA), nil).Once()
		engine.EXPECT().AddMagnet(mock.Anything, "magnet:?xt=urn:btih:"+hashB).Return(torrent.InfoHash(hashB), nil).Once()
		engine.EXPECT().Pause(mock.Anything, torrent.InfoHash(hashB)).Return(nil).Once()

		require.NoError(t, svc.Restore(t.Context()))
	})

	t.Run("repository error", func(t *testing.T) {
		engine, repo := newServiceFixture(t)
		svc := service.NewTorrentService(newTestLogger(t), engine, repo)

		_ = engine
		repo.EXPECT().List(mock.Anything).Return(nil, errors.New("db boom")).Once()

		err := svc.Restore(t.Context())
		assert.ErrorContains(t, err, "failed to list torrents")
	})

	t.Run("individual failure logs and continues", func(t *testing.T) {
		engine, repo := newServiceFixture(t)
		svc := service.NewTorrentService(newTestLogger(t), engine, repo)

		hashA := "1111111111111111111111111111111111111111"
		hashB := "2222222222222222222222222222222222222222"

		repo.EXPECT().List(mock.Anything).Return([]database.Torrent{
			{InfoHash: hashA, Magnet: "magnet:?xt=urn:btih:" + hashA},
			{InfoHash: hashB, Magnet: "magnet:?xt=urn:btih:" + hashB},
		}, nil).Once()
		engine.EXPECT().AddMagnet(mock.Anything, "magnet:?xt=urn:btih:"+hashA).Return("", errors.New("dead")).Once()
		engine.EXPECT().AddMagnet(mock.Anything, "magnet:?xt=urn:btih:"+hashB).Return(torrent.InfoHash(hashB), nil).Once()

		require.NoError(t, svc.Restore(t.Context()))
	})
}

func newServiceFixture(t *testing.T) (*MockTorrentEngine, *MockTorrentRepository) {
	t.Helper()

	return NewMockTorrentEngine(t), NewMockTorrentRepository(t)
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

