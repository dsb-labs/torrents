// Package torrent provides an Engine wrapping anacrolix/torrent for use by the
// torrents server.
package torrent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"

	"github.com/anacrolix/torrent/metainfo"
)

// ErrNotFound is returned when no torrent with the given info hash is
// tracked by the engine.
var ErrNotFound = errors.New("torrent not found")

type (
	// The InfoHash type is a torrent's 40-character hex BitTorrent info hash.
	InfoHash string

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
		logger *slog.Logger
		client Client
	}

	// The Config type contains fields used to construct an Engine.
	Config struct {
		// The logger used for engine lifecycle events.
		Logger *slog.Logger
		// The torrent client used to perform operations.
		Client Client
	}
)

// New returns an Engine that operates against the Client in config.
func New(config Config) *Engine {
	return &Engine{
		logger: config.Logger.With("component", "engine"),
		client: config.Client,
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

	t.DownloadAll()

	return InfoHash(t.InfoHash().HexString()), nil
}

// AddFile adds a torrent from a .torrent metainfo file read from r.
func (e *Engine) AddFile(ctx context.Context, r io.Reader) (InfoHash, error) {
	mi, err := metainfo.Load(r)
	if err != nil {
		return "", fmt.Errorf("failed to parse torrent file: %w", err)
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

	t.DownloadAll()

	return InfoHash(t.InfoHash().HexString()), nil
}

// Remove stops tracking the torrent identified by hash. Downloaded files are
// left on disk. Returns ErrNotFound when the engine isn't tracking the
// given torrent.
func (e *Engine) Remove(ctx context.Context, hash InfoHash) error {
	t, ok, err := e.find(hash)
	switch {
	case err != nil:
		return err
	case !ok:
		return ErrNotFound
	}

	t.Drop()

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

func (e *Engine) find(hash InfoHash) (Torrent, bool, error) {
	var h metainfo.Hash
	if err := h.FromHexString(string(hash)); err != nil {
		return nil, false, fmt.Errorf("invalid info hash %q: %w", hash, err)
	}

	t, ok := e.client.Torrent(h)
	return t, ok, nil
}
