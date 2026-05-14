package torrent

import (
	"errors"
	"fmt"

	"github.com/anacrolix/chansync/events"
	anacrolix "github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
)

type (
	// The Client interface describes the torrent-client surface that Engine uses.
	Client interface {
		// AddMagnet should add a torrent identified by the given magnet URI.
		AddMagnet(uri string) (Torrent, error)
		// AddTorrent should add a torrent from the given metainfo.
		AddTorrent(mi *metainfo.MetaInfo) (Torrent, error)
		// Torrent should return the tracked torrent with the given info hash,
		// or (nil, false) when no such torrent is tracked.
		Torrent(hash metainfo.Hash) (Torrent, bool)
		// Close should shut down the client and release its resources.
		Close() error
	}

	// The Torrent interface describes the operations Engine performs on an
	// individual torrent in the client.
	Torrent interface {
		// GotInfo should return a channel that closes once the torrent's
		// metainfo has been received.
		GotInfo() events.Done
		// DownloadAll should start downloading every file in the torrent.
		DownloadAll()
		// Drop should remove the torrent from the client.
		Drop()
		// InfoHash should return the torrent's info hash.
		InfoHash() metainfo.Hash
		// Name should return the torrent's display name.
		Name() string
		// Length should return the total length of the torrent's content.
		Length() int64
		// BytesCompleted should return how many bytes of the content have been downloaded.
		BytesCompleted() int64
		// Stats should return live peer / transfer statistics.
		Stats() anacrolix.TorrentStats
	}

	anacrolixClient struct {
		inner *anacrolix.Client
	}
)

// NewClient returns a Client backed by anacrolix/torrent, writing downloaded
// content under dataDir. The caller owns the returned Client's lifecycle and
// must Close it when no longer needed.
func NewClient(dataDir string) (Client, error) {
	if dataDir == "" {
		return nil, errors.New("data directory is required")
	}

	cfg := anacrolix.NewDefaultClientConfig()
	cfg.DataDir = dataDir

	inner, err := anacrolix.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create torrent client: %w", err)
	}

	return &anacrolixClient{inner: inner}, nil
}

func (a *anacrolixClient) AddMagnet(uri string) (Torrent, error) {
	t, err := a.inner.AddMagnet(uri)
	if err != nil {
		return nil, err
	}

	return t, nil
}

func (a *anacrolixClient) AddTorrent(mi *metainfo.MetaInfo) (Torrent, error) {
	t, err := a.inner.AddTorrent(mi)
	if err != nil {
		return nil, err
	}

	return t, nil
}

func (a *anacrolixClient) Torrent(hash metainfo.Hash) (Torrent, bool) {
	t, ok := a.inner.Torrent(hash)
	if !ok {
		return nil, false
	}

	return t, true
}

func (a *anacrolixClient) Close() error {
	if errs := a.inner.Close(); len(errs) > 0 {
		return errors.Join(errs...)
	}

	return nil
}
