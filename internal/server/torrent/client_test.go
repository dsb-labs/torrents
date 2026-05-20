package torrent_test

import (
	"testing"

	"github.com/anacrolix/torrent/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dsb-labs/torrents/internal/server/torrent"
)

func TestNewClient(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		client, err := torrent.NewClient(newTestLogger(t), t.TempDir(), storage.NewMapPieceCompletion())
		require.NoError(t, err)

		require.NoError(t, client.Close())
	})

	t.Run("missing data dir", func(t *testing.T) {
		_, err := torrent.NewClient(newTestLogger(t), "", storage.NewMapPieceCompletion())
		assert.ErrorContains(t, err, "data directory")
	})

	t.Run("missing piece completion", func(t *testing.T) {
		_, err := torrent.NewClient(newTestLogger(t), t.TempDir(), nil)
		assert.ErrorContains(t, err, "piece completion")
	})
}
