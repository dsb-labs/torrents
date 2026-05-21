// Package torrent provides an Engine wrapping anacrolix/torrent for use by the
// torrents server.
package torrent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	anacrolix "github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
)

var (
	// ErrNotFound is returned when no torrent with the given info hash is
	// tracked by the engine.
	ErrNotFound = errors.New("torrent not found")
	// ErrInvalidFile is returned when AddFile is given data that does not
	// parse as a valid .torrent metainfo file.
	ErrInvalidFile = errors.New("invalid torrent file")
)

type (
	// The InfoHash type is a torrent's 40-character hex BitTorrent info hash.
	InfoHash string

	// The FileProgress type describes the live state of a single file in a torrent.
	FileProgress struct {
		// The file's path within the torrent.
		Path string
		// The total length of the file, in bytes.
		Length int64
		// How many bytes of the file have been downloaded.
		BytesCompleted int64
	}

	// The Progress type describes the live state of a torrent.
	Progress struct {
		// The info hash that identifies the torrent.
		InfoHash InfoHash
		// The display name reported in the torrent's metainfo.
		Name string
		// The total length of the torrent's content, in bytes.
		Length int64
		// How many bytes of the content have been downloaded.
		BytesCompleted int64
		// The number of peers the client is currently connected to.
		ActivePeers int
		// The number of those peers known to be seeders.
		Seeders int
	}

	// The Engine type provides info-hash-keyed operations against a torrent client.
	Engine struct {
		logger  *slog.Logger
		client  Client
		dataDir string
	}

	// The Config type contains fields used to construct an Engine.
	Config struct {
		// The logger used for engine lifecycle events.
		Logger *slog.Logger
		// The torrent client used to perform operations.
		Client Client
		// The data directory under which downloaded content is written.
		// Used to locate on-disk files when Remove is asked to delete them.
		DataDir string
	}
)

// New returns an Engine that operates against the Client in config.
func New(config Config) *Engine {
	return &Engine{
		logger:  config.Logger.With("component", "engine"),
		client:  config.Client,
		dataDir: config.DataDir,
	}
}

// AddMagnet adds a torrent identified by the given magnet URI. The call blocks
// until the torrent's metainfo has been received or ctx is cancelled.
func (e *Engine) AddMagnet(ctx context.Context, uri string) (InfoHash, error) {
	t, err := e.client.AddMagnet(uri)
	if err != nil {
		return "", fmt.Errorf("failed to add magnet: %w", err)
	}

	select {
	case <-t.GotInfo():
	case <-ctx.Done():
		t.Drop()
		return "", ctx.Err()
	}

	if err = t.VerifyDataContext(ctx); err != nil {
		t.Drop()
		return "", fmt.Errorf("failed to verify torrent data: %w", err)
	}

	t.DownloadAll()

	return InfoHash(t.InfoHash().HexString()), nil
}

// AddFile adds a torrent from a .torrent metainfo file read from r.
func (e *Engine) AddFile(ctx context.Context, r io.Reader) (InfoHash, error) {
	mi, err := metainfo.Load(r)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidFile, err)
	}

	t, err := e.client.AddTorrent(mi)
	if err != nil {
		return "", fmt.Errorf("failed to add torrent: %w", err)
	}

	select {
	case <-t.GotInfo():
	case <-ctx.Done():
		t.Drop()
		return "", ctx.Err()
	}

	if err = t.VerifyDataContext(ctx); err != nil {
		t.Drop()
		return "", fmt.Errorf("failed to verify torrent data: %w", err)
	}

	t.DownloadAll()

	return InfoHash(t.InfoHash().HexString()), nil
}

// Remove stops tracking the torrent identified by hash. When deleteFiles
// is false, downloaded content is left on disk; when true, the torrent's
// content directory under the engine's data dir is removed. Returns
// ErrNotFound when the engine isn't tracking the given torrent.
func (e *Engine) Remove(ctx context.Context, hash InfoHash, deleteFiles bool) error {
	t, ok, err := e.find(hash)
	switch {
	case err != nil:
		return err
	case !ok:
		return ErrNotFound
	}

	name := t.Name()
	t.Drop()

	if deleteFiles && name != "" {
		// Anacrolix's default file storage uses ".part" suffixes for
		// not-yet-completed pieces, which for single-file torrents
		// sit alongside (not under) the torrent's content path —
		// hence the second RemoveAll. For multi-file torrents the
		// .part files live inside <dataDir>/<name>/ and the first
		// RemoveAll covers them.
		for _, path := range []string{filepath.Join(e.dataDir, name), filepath.Join(e.dataDir, name+".part")} {
			if err = os.RemoveAll(path); err != nil {
				return fmt.Errorf("failed to remove torrent data: %w", err)
			}
		}
	}

	return nil
}

// Snapshot returns the current state of the torrent identified by hash.
// Returns ErrNotFound when the engine isn't tracking the given torrent.
func (e *Engine) Snapshot(hash InfoHash) (Progress, error) {
	t, ok, err := e.find(hash)
	switch {
	case err != nil:
		return Progress{}, err
	case !ok:
		return Progress{}, ErrNotFound
	}

	stats := t.Stats()

	return Progress{
		InfoHash:       hash,
		Name:           t.Name(),
		Length:         t.Length(),
		BytesCompleted: t.BytesCompleted(),
		ActivePeers:    stats.ActivePeers,
		Seeders:        stats.ConnectedSeeders,
	}, nil
}

// Files returns the per-file progress for the torrent identified by hash.
// Returns ErrNotFound when the engine isn't tracking the given torrent or when
// file-level data is unavailable.
func (e *Engine) Files(hash InfoHash) ([]FileProgress, error) {
	t, ok, err := e.find(hash)
	switch {
	case err != nil:
		return nil, err
	case !ok:
		return nil, ErrNotFound
	}

	type filer interface {
		Files() []*anacrolix.File
	}

	f, ok := t.(filer)
	if !ok {
		return nil, ErrNotFound
	}

	files := f.Files()
	progress := make([]FileProgress, len(files))
	for i, file := range files {
		progress[i] = FileProgress{
			Path:           file.DisplayPath(),
			Length:         file.Length(),
			BytesCompleted: file.BytesCompleted(),
		}
	}

	return progress, nil
}

func (e *Engine) find(hash InfoHash) (Torrent, bool, error) {
	var h metainfo.Hash
	if err := h.FromHexString(string(hash)); err != nil {
		return nil, false, fmt.Errorf("invalid info hash %q: %w", hash, err)
	}

	t, ok := e.client.Torrent(h)
	return t, ok, nil
}
