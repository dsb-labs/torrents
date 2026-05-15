package torrent_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"

	anacrolix "github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/dsb-labs/torrents/internal/server/torrent"
)

const testInfoHashHex = "0123456789abcdef0123456789abcdef01234567"

func TestEngine_AddMagnet(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		mockClient := NewMockClient(t)
		mockTorrent := newMockTorrentWithInfoReady(t)

		mockClient.EXPECT().AddMagnet("magnet:?xt=urn:btih:"+testInfoHashHex).Return(mockTorrent, nil).Once()
		mockTorrent.EXPECT().DownloadAll().Return().Once()

		engine := torrent.New(torrent.Config{
			Logger: newTestLogger(t),
			Client: mockClient,
		})

		hash, err := engine.AddMagnet(t.Context(), "magnet:?xt=urn:btih:"+testInfoHashHex)
		require.NoError(t, err)
		assert.Equal(t, torrent.InfoHash(testInfoHashHex), hash)
	})

	t.Run("client error", func(t *testing.T) {
		mockClient := NewMockClient(t)
		mockClient.EXPECT().AddMagnet("bad").Return(nil, errors.New("invalid uri")).Once()

		engine := torrent.New(torrent.Config{
			Logger: newTestLogger(t),
			Client: mockClient,
		})

		_, err := engine.AddMagnet(t.Context(), "bad")
		assert.ErrorContains(t, err, "invalid uri")
	})

	t.Run("context cancelled", func(t *testing.T) {
		mockClient := NewMockClient(t)
		mockTorrent := NewMockTorrent(t)

		never := make(chan struct{})
		mockClient.EXPECT().AddMagnet("magnet:?xt=urn:btih:"+testInfoHashHex).Return(mockTorrent, nil).Once()
		mockTorrent.EXPECT().GotInfo().Return(never).Once()
		mockTorrent.EXPECT().Drop().Return().Once()

		engine := torrent.New(torrent.Config{
			Logger: newTestLogger(t),
			Client: mockClient,
		})

		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		_, err := engine.AddMagnet(ctx, "magnet:?xt=urn:btih:"+testInfoHashHex)
		assert.ErrorIs(t, err, context.Canceled)
	})
}

func TestEngine_AddFile(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		mockClient := NewMockClient(t)
		mockTorrent := newMockTorrentWithInfoReady(t)

		mockClient.EXPECT().AddTorrent(mock.Anything).Return(mockTorrent, nil).Once()
		mockTorrent.EXPECT().DownloadAll().Return().Once()

		engine := torrent.New(torrent.Config{
			Logger: newTestLogger(t),
			Client: mockClient,
		})

		hash, err := engine.AddFile(t.Context(), bytes.NewReader(testTorrentFile(t)))
		require.NoError(t, err)
		assert.Equal(t, torrent.InfoHash(testInfoHashHex), hash)
	})

	t.Run("invalid metainfo", func(t *testing.T) {
		engine := torrent.New(torrent.Config{
			Logger: newTestLogger(t),
			Client: NewMockClient(t),
		})

		_, err := engine.AddFile(t.Context(), bytes.NewReader([]byte("not a torrent file")))
		assert.ErrorContains(t, err, "failed to parse torrent file")
	})
}

func TestEngine_Remove(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		mockClient := NewMockClient(t)
		mockTorrent := NewMockTorrent(t)

		mockClient.EXPECT().Torrent(testInfoHash()).Return(mockTorrent, true).Once()
		mockTorrent.EXPECT().Drop().Return().Once()

		engine := torrent.New(torrent.Config{
			Logger: newTestLogger(t),
			Client: mockClient,
		})

		require.NoError(t, engine.Remove(t.Context(), testInfoHashHex))
	})

	t.Run("not found", func(t *testing.T) {
		mockClient := NewMockClient(t)
		mockClient.EXPECT().Torrent(testInfoHash()).Return(nil, false).Once()

		engine := torrent.New(torrent.Config{
			Logger: newTestLogger(t),
			Client: mockClient,
		})

		err := engine.Remove(t.Context(), testInfoHashHex)
		assert.ErrorIs(t, err, torrent.ErrNotFound)
	})

	t.Run("invalid hash", func(t *testing.T) {
		engine := torrent.New(torrent.Config{
			Logger: newTestLogger(t),
			Client: NewMockClient(t),
		})

		err := engine.Remove(t.Context(), "not-a-valid-hash")
		assert.ErrorContains(t, err, "invalid info hash")
	})
}

func TestEngine_Snapshot(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		mockClient := NewMockClient(t)
		mockTorrent := NewMockTorrent(t)

		mockClient.EXPECT().Torrent(testInfoHash()).Return(mockTorrent, true).Once()
		mockTorrent.EXPECT().Name().Return("debian.iso").Once()
		mockTorrent.EXPECT().Length().Return(int64(1_000_000)).Once()
		mockTorrent.EXPECT().BytesCompleted().Return(int64(250_000)).Once()
		mockTorrent.EXPECT().Stats().Return(anacrolix.TorrentStats{
			TorrentGauges: anacrolix.TorrentGauges{
				ActivePeers:      8,
				ConnectedSeeders: 3,
			},
		}).Once()

		engine := torrent.New(torrent.Config{
			Logger: newTestLogger(t),
			Client: mockClient,
		})

		progress, err := engine.Snapshot(testInfoHashHex)
		require.NoError(t, err)
		assert.Equal(t, torrent.Progress{
			InfoHash:       testInfoHashHex,
			Name:           "debian.iso",
			Length:         1_000_000,
			BytesCompleted: 250_000,
			ActivePeers:    8,
			Seeders:        3,
		}, progress)
	})

	t.Run("not found", func(t *testing.T) {
		mockClient := NewMockClient(t)
		mockClient.EXPECT().Torrent(testInfoHash()).Return(nil, false).Once()

		engine := torrent.New(torrent.Config{
			Logger: newTestLogger(t),
			Client: mockClient,
		})

		_, err := engine.Snapshot(testInfoHashHex)
		assert.ErrorIs(t, err, torrent.ErrNotFound)
	})
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

func testInfoHash() metainfo.Hash {
	var h metainfo.Hash
	_ = h.FromHexString(testInfoHashHex)
	return h
}

func newMockTorrentWithInfoReady(t *testing.T) *MockTorrent {
	t.Helper()

	mt := NewMockTorrent(t)

	ready := make(chan struct{})
	close(ready)
	mt.EXPECT().GotInfo().Return(ready).Once()
	mt.EXPECT().VerifyDataContext(mock.Anything).Return(nil).Once()
	mt.EXPECT().InfoHash().Return(testInfoHash()).Once()

	return mt
}

func testTorrentFile(t *testing.T) []byte {
	t.Helper()

	mi := metainfo.MetaInfo{
		InfoBytes: []byte("d4:name8:test.bin12:piece lengthi16384e6:pieces20:" + string(make([]byte, 20)) + "6:lengthi16384ee"),
	}

	var buf bytes.Buffer
	require.NoError(t, mi.Write(&buf))

	return buf.Bytes()
}
