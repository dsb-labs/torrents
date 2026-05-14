package service_test

import (
	"bytes"
	"errors"
	"io"
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

	tt := []struct {
		Name              string
		URI               string
		Options           service.AddOptions
		SetupMocks        func(*MockTorrentEngine, *MockTorrentRepository)
		Assert            func(*testing.T, service.Torrent)
		ExpectErr         error
		ExpectErrContains string
	}{
		{
			Name:    "success",
			URI:     "magnet:?xt=urn:btih:" + testInfoHash,
			Options: service.AddOptions{Label: "linux iso", TargetDir: "/data/downloads"},
			SetupMocks: func(engine *MockTorrentEngine, repo *MockTorrentRepository) {
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
					InfoHash:    testInfoHash,
					Name:        "linux-amd64.iso",
					Length:      500_000_000,
					ActivePeers: 2,
					Seeders:     1,
				}, nil).Once()
			},
			Assert: func(t *testing.T, got service.Torrent) {
				assert.Equal(t, testInfoHash, got.InfoHash)
				assert.Equal(t, "linux iso", got.Label)
				assert.Equal(t, "linux-amd64.iso", got.Name)
				assert.EqualValues(t, 500_000_000, got.Length)
				assert.Equal(t, 2, got.ActivePeers)
			},
		},
		{
			Name: "duplicate",
			URI:  "magnet:?xt=urn:btih:" + testInfoHash,
			SetupMocks: func(engine *MockTorrentEngine, repo *MockTorrentRepository) {
				engine.EXPECT().AddMagnet(mock.Anything, mock.Anything).Return(testInfoHash, nil).Once()
				repo.EXPECT().Create(mock.Anything, mock.Anything).Return(database.ErrTorrentAlreadyExists).Once()
			},
			ExpectErr: service.ErrTorrentAlreadyExists,
		},
		{
			Name: "engine error",
			URI:  "bad",
			SetupMocks: func(engine *MockTorrentEngine, repo *MockTorrentRepository) {
				engine.EXPECT().AddMagnet(mock.Anything, mock.Anything).Return("", errors.New("invalid uri")).Once()
			},
			ExpectErrContains: "failed to add magnet",
		},
	}

	for _, tc := range tt {
		t.Run(tc.Name, func(t *testing.T) {
			svc, engine, repo := newServiceFixture(t)
			if tc.SetupMocks != nil {
				tc.SetupMocks(engine, repo)
			}

			got, err := svc.AddMagnet(t.Context(), tc.URI, tc.Options)
			if assertExpectedErr(t, err, tc.ExpectErr, tc.ExpectErrContains) {
				return
			}

			require.NoError(t, err)
			if tc.Assert != nil {
				tc.Assert(t, got)
			}
		})
	}
}

func TestTorrentService_AddFile(t *testing.T) {
	t.Parallel()

	tt := []struct {
		Name       string
		Body       io.Reader
		Options    service.AddOptions
		SetupMocks func(*MockTorrentEngine, *MockTorrentRepository)
		Assert     func(*testing.T, service.Torrent)
		ExpectErr  error
	}{
		{
			Name: "success",
			Body: bytes.NewReader([]byte("fake-torrent")),
			SetupMocks: func(engine *MockTorrentEngine, repo *MockTorrentRepository) {
				engine.EXPECT().AddFile(mock.Anything, mock.Anything).Return(testInfoHash, nil).Once()
				repo.EXPECT().Create(mock.Anything, mock.MatchedBy(func(t database.Torrent) bool {
					return t.InfoHash == testInfoHash && t.Magnet == "magnet:?xt=urn:btih:"+testInfoHash
				})).Return(nil).Once()
				repo.EXPECT().Get(mock.Anything, testInfoHash).Return(database.Torrent{InfoHash: testInfoHash}, nil).Once()
				engine.EXPECT().Snapshot(torrent.InfoHash(testInfoHash)).Return(torrent.Progress{}, torrent.ErrNotFound).Once()
			},
			Assert: func(t *testing.T, got service.Torrent) {
				assert.Equal(t, testInfoHash, got.InfoHash)
			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.Name, func(t *testing.T) {
			svc, engine, repo := newServiceFixture(t)
			if tc.SetupMocks != nil {
				tc.SetupMocks(engine, repo)
			}

			got, err := svc.AddFile(t.Context(), tc.Body, tc.Options)
			if assertExpectedErr(t, err, tc.ExpectErr, "") {
				return
			}

			require.NoError(t, err)
			if tc.Assert != nil {
				tc.Assert(t, got)
			}
		})
	}
}

func TestTorrentService_Get(t *testing.T) {
	t.Parallel()

	tt := []struct {
		Name       string
		SetupMocks func(*MockTorrentEngine, *MockTorrentRepository)
		Assert     func(*testing.T, service.Torrent)
		ExpectErr  error
	}{
		{
			Name: "success with live state",
			SetupMocks: func(engine *MockTorrentEngine, repo *MockTorrentRepository) {
				repo.EXPECT().Get(mock.Anything, testInfoHash).Return(database.Torrent{
					InfoHash:  testInfoHash,
					Magnet:    "magnet:?xt=urn:btih:" + testInfoHash,
					Label:     "iso",
					TargetDir: "/data",
					CreatedAt: time.Unix(1700000000, 0).UTC(),
					UpdatedAt: time.Unix(1700000000, 0).UTC(),
				}, nil).Once()
				engine.EXPECT().Snapshot(torrent.InfoHash(testInfoHash)).Return(torrent.Progress{
					Name:           "thing.iso",
					Length:         1024,
					BytesCompleted: 256,
					ActivePeers:    3,
					Seeders:        1,
				}, nil).Once()
			},
			Assert: func(t *testing.T, got service.Torrent) {
				assert.Equal(t, "thing.iso", got.Name)
				assert.EqualValues(t, 1024, got.Length)
				assert.EqualValues(t, 256, got.BytesCompleted)
			},
		},
		{
			Name: "success engine missing",
			SetupMocks: func(engine *MockTorrentEngine, repo *MockTorrentRepository) {
				repo.EXPECT().Get(mock.Anything, testInfoHash).Return(database.Torrent{InfoHash: testInfoHash}, nil).Once()
				engine.EXPECT().Snapshot(torrent.InfoHash(testInfoHash)).Return(torrent.Progress{}, torrent.ErrNotFound).Once()
			},
			Assert: func(t *testing.T, got service.Torrent) {
				assert.Equal(t, testInfoHash, got.InfoHash)
				assert.Empty(t, got.Name)
			},
		},
		{
			Name: "not found",
			SetupMocks: func(engine *MockTorrentEngine, repo *MockTorrentRepository) {
				repo.EXPECT().Get(mock.Anything, testInfoHash).Return(database.Torrent{}, database.ErrTorrentNotFound).Once()
			},
			ExpectErr: service.ErrTorrentNotFound,
		},
	}

	for _, tc := range tt {
		t.Run(tc.Name, func(t *testing.T) {
			svc, engine, repo := newServiceFixture(t)
			if tc.SetupMocks != nil {
				tc.SetupMocks(engine, repo)
			}

			got, err := svc.Get(t.Context(), testInfoHash)
			if assertExpectedErr(t, err, tc.ExpectErr, "") {
				return
			}

			require.NoError(t, err)
			if tc.Assert != nil {
				tc.Assert(t, got)
			}
		})
	}
}

func TestTorrentService_List(t *testing.T) {
	t.Parallel()

	tt := []struct {
		Name       string
		SetupMocks func(*MockTorrentEngine, *MockTorrentRepository)
		Assert     func(*testing.T, []service.Torrent)
		ExpectErr  error
	}{
		{
			Name: "empty",
			SetupMocks: func(engine *MockTorrentEngine, repo *MockTorrentRepository) {
				repo.EXPECT().List(mock.Anything).Return(nil, nil).Once()
			},
			Assert: func(t *testing.T, got []service.Torrent) {
				assert.Empty(t, got)
			},
		},
		{
			Name: "multiple",
			SetupMocks: func(engine *MockTorrentEngine, repo *MockTorrentRepository) {
				hashA := "1111111111111111111111111111111111111111"
				hashB := "2222222222222222222222222222222222222222"

				repo.EXPECT().List(mock.Anything).Return([]database.Torrent{
					{InfoHash: hashA},
					{InfoHash: hashB},
				}, nil).Once()
				engine.EXPECT().Snapshot(torrent.InfoHash(hashA)).Return(torrent.Progress{Name: "A", BytesCompleted: 10}, nil).Once()
				engine.EXPECT().Snapshot(torrent.InfoHash(hashB)).Return(torrent.Progress{}, torrent.ErrNotFound).Once()
			},
			Assert: func(t *testing.T, got []service.Torrent) {
				require.Len(t, got, 2)
				assert.Equal(t, "A", got[0].Name)
				assert.EqualValues(t, 10, got[0].BytesCompleted)
				assert.Empty(t, got[1].Name)
			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.Name, func(t *testing.T) {
			svc, engine, repo := newServiceFixture(t)
			if tc.SetupMocks != nil {
				tc.SetupMocks(engine, repo)
			}

			got, err := svc.List(t.Context())
			if assertExpectedErr(t, err, tc.ExpectErr, "") {
				return
			}

			require.NoError(t, err)
			if tc.Assert != nil {
				tc.Assert(t, got)
			}
		})
	}
}

func TestTorrentService_Remove(t *testing.T) {
	t.Parallel()

	tt := []struct {
		Name       string
		SetupMocks func(*MockTorrentEngine, *MockTorrentRepository)
		ExpectErr  error
	}{
		{
			Name: "success",
			SetupMocks: func(engine *MockTorrentEngine, repo *MockTorrentRepository) {
				engine.EXPECT().Remove(mock.Anything, torrent.InfoHash(testInfoHash)).Return(nil).Once()
				repo.EXPECT().Delete(mock.Anything, testInfoHash).Return(nil).Once()
			},
		},
		{
			Name: "not found in engine still deletes row",
			SetupMocks: func(engine *MockTorrentEngine, repo *MockTorrentRepository) {
				engine.EXPECT().Remove(mock.Anything, torrent.InfoHash(testInfoHash)).Return(torrent.ErrNotFound).Once()
				repo.EXPECT().Delete(mock.Anything, testInfoHash).Return(nil).Once()
			},
		},
		{
			Name: "not found",
			SetupMocks: func(engine *MockTorrentEngine, repo *MockTorrentRepository) {
				engine.EXPECT().Remove(mock.Anything, torrent.InfoHash(testInfoHash)).Return(torrent.ErrNotFound).Once()
				repo.EXPECT().Delete(mock.Anything, testInfoHash).Return(database.ErrTorrentNotFound).Once()
			},
			ExpectErr: service.ErrTorrentNotFound,
		},
	}

	for _, tc := range tt {
		t.Run(tc.Name, func(t *testing.T) {
			svc, engine, repo := newServiceFixture(t)
			if tc.SetupMocks != nil {
				tc.SetupMocks(engine, repo)
			}

			err := svc.Remove(t.Context(), testInfoHash)
			if assertExpectedErr(t, err, tc.ExpectErr, "") {
				return
			}

			require.NoError(t, err)
		})
	}
}

func TestTorrentService_Pause(t *testing.T) {
	t.Parallel()

	tt := []struct {
		Name       string
		SetupMocks func(*MockTorrentEngine, *MockTorrentRepository)
		ExpectErr  error
	}{
		{
			Name: "success",
			SetupMocks: func(engine *MockTorrentEngine, repo *MockTorrentRepository) {
				engine.EXPECT().Pause(mock.Anything, torrent.InfoHash(testInfoHash)).Return(nil).Once()
				repo.EXPECT().SetPaused(mock.Anything, testInfoHash, true).Return(nil).Once()
			},
		},
		{
			Name: "not found in engine",
			SetupMocks: func(engine *MockTorrentEngine, repo *MockTorrentRepository) {
				engine.EXPECT().Pause(mock.Anything, torrent.InfoHash(testInfoHash)).Return(torrent.ErrNotFound).Once()
			},
			ExpectErr: service.ErrTorrentNotFound,
		},
	}

	for _, tc := range tt {
		t.Run(tc.Name, func(t *testing.T) {
			svc, engine, repo := newServiceFixture(t)
			if tc.SetupMocks != nil {
				tc.SetupMocks(engine, repo)
			}

			err := svc.Pause(t.Context(), testInfoHash)
			if assertExpectedErr(t, err, tc.ExpectErr, "") {
				return
			}

			require.NoError(t, err)
		})
	}
}

func TestTorrentService_Resume(t *testing.T) {
	t.Parallel()

	tt := []struct {
		Name       string
		SetupMocks func(*MockTorrentEngine, *MockTorrentRepository)
		ExpectErr  error
	}{
		{
			Name: "success",
			SetupMocks: func(engine *MockTorrentEngine, repo *MockTorrentRepository) {
				engine.EXPECT().Resume(mock.Anything, torrent.InfoHash(testInfoHash)).Return(nil).Once()
				repo.EXPECT().SetPaused(mock.Anything, testInfoHash, false).Return(nil).Once()
			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.Name, func(t *testing.T) {
			svc, engine, repo := newServiceFixture(t)
			if tc.SetupMocks != nil {
				tc.SetupMocks(engine, repo)
			}

			err := svc.Resume(t.Context(), testInfoHash)
			if assertExpectedErr(t, err, tc.ExpectErr, "") {
				return
			}

			require.NoError(t, err)
		})
	}
}

func TestTorrentService_Restore(t *testing.T) {
	t.Parallel()

	hashA := "1111111111111111111111111111111111111111"
	hashB := "2222222222222222222222222222222222222222"

	tt := []struct {
		Name              string
		SetupMocks        func(*MockTorrentEngine, *MockTorrentRepository)
		ExpectErr         error
		ExpectErrContains string
	}{
		{
			Name: "success",
			SetupMocks: func(engine *MockTorrentEngine, repo *MockTorrentRepository) {
				repo.EXPECT().List(mock.Anything).Return([]database.Torrent{
					{InfoHash: hashA, Magnet: "magnet:?xt=urn:btih:" + hashA, Paused: false},
					{InfoHash: hashB, Magnet: "magnet:?xt=urn:btih:" + hashB, Paused: true},
				}, nil).Once()
				engine.EXPECT().AddMagnet(mock.Anything, "magnet:?xt=urn:btih:"+hashA).Return(torrent.InfoHash(hashA), nil).Once()
				engine.EXPECT().AddMagnet(mock.Anything, "magnet:?xt=urn:btih:"+hashB).Return(torrent.InfoHash(hashB), nil).Once()
				engine.EXPECT().Pause(mock.Anything, torrent.InfoHash(hashB)).Return(nil).Once()
			},
		},
		{
			Name: "repository error",
			SetupMocks: func(engine *MockTorrentEngine, repo *MockTorrentRepository) {
				repo.EXPECT().List(mock.Anything).Return(nil, errors.New("db boom")).Once()
			},
			ExpectErrContains: "failed to list torrents",
		},
		{
			Name: "individual failure logs and continues",
			SetupMocks: func(engine *MockTorrentEngine, repo *MockTorrentRepository) {
				repo.EXPECT().List(mock.Anything).Return([]database.Torrent{
					{InfoHash: hashA, Magnet: "magnet:?xt=urn:btih:" + hashA},
					{InfoHash: hashB, Magnet: "magnet:?xt=urn:btih:" + hashB},
				}, nil).Once()
				engine.EXPECT().AddMagnet(mock.Anything, "magnet:?xt=urn:btih:"+hashA).Return("", errors.New("dead")).Once()
				engine.EXPECT().AddMagnet(mock.Anything, "magnet:?xt=urn:btih:"+hashB).Return(torrent.InfoHash(hashB), nil).Once()
			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.Name, func(t *testing.T) {
			svc, engine, repo := newServiceFixture(t)
			if tc.SetupMocks != nil {
				tc.SetupMocks(engine, repo)
			}

			err := svc.Restore(t.Context())
			if assertExpectedErr(t, err, tc.ExpectErr, tc.ExpectErrContains) {
				return
			}

			require.NoError(t, err)
		})
	}
}

func newServiceFixture(t *testing.T) (*service.TorrentService, *MockTorrentEngine, *MockTorrentRepository) {
	t.Helper()

	engine := NewMockTorrentEngine(t)
	repo := NewMockTorrentRepository(t)
	svc := service.NewTorrentService(newTestLogger(t), engine, repo)

	return svc, engine, repo
}

func assertExpectedErr(t *testing.T, err, sentinel error, contains string) bool {
	t.Helper()

	switch {
	case sentinel != nil:
		assert.ErrorIs(t, err, sentinel)
		return true
	case contains != "":
		assert.ErrorContains(t, err, contains)
		return true
	}

	return false
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
