package torrent_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dsb-labs/torrents/internal/server/torrent"
)

func TestNewClient(t *testing.T) {
	t.Parallel()

	client, err := torrent.NewClient(t.TempDir())
	require.NoError(t, err)

	require.NoError(t, client.Close())
}
