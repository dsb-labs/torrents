package torrent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/anacrolix/chansync/events"
	anacrolixlog "github.com/anacrolix/log"
	anacrolix "github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/anacrolix/torrent/storage"
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
		// VerifyDataContext should re-hash every piece against the
		// on-disk data and update completion state accordingly.
		VerifyDataContext(ctx context.Context) error
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
//
// The supplied PieceCompletion is used to persist piece state across the
// drop-and-re-add pause/resume cycle and across server restarts. The caller
// owns its lifecycle.
//
// Every line anacrolix/torrent emits is funnelled through the supplied
// logger at debug level, regardless of the level anacrolix originally
// chose. The application's slog level controls whether any of it
// surfaces — at the default info level the runtime stays quiet, flip to
// debug to see it.
func NewClient(logger *slog.Logger, dataDir string, completion storage.PieceCompletion) (Client, error) {
	if dataDir == "" {
		return nil, errors.New("data directory is required")
	}
	if completion == nil {
		return nil, errors.New("piece completion store is required")
	}

	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}

	debugWriter := slog.NewLogLogger(logger.With("component", "anacrolix").Handler(), slog.LevelDebug).Writer()
	al := anacrolixlog.NewLogger()
	al.SetHandlers(anacrolixlog.StreamHandler{
		W:   debugWriter,
		Fmt: anacrolixlog.LineFormatter,
	})

	cfg := anacrolix.NewDefaultClientConfig()
	cfg.DataDir = dataDir
	cfg.DefaultStorage = storage.NewFileWithCompletion(dataDir, completion)
	cfg.Logger = al
	// Anacrolix 1.61.0's webseed scheduler panics with `panicif.False`
	// from updateWebseedRequests when an in-progress torrent is dropped
	// while the timer is mid-iteration, taking down the whole process.
	cfg.DisableWebseeds = true

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
