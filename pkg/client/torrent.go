package client

import (
	"context"
	"net/http"
	"path"
	"time"

	"github.com/dsb-labs/torrents/internal/server/api"
)

// The Torrent type is the client-side view of a managed torrent.
type Torrent struct {
	// The torrent's info hash.
	InfoHash string
	// The magnet URI the torrent was added with.
	Magnet string
	// A human-supplied label, or empty when unset.
	Label string
	// The filesystem directory the torrent's content is written into.
	TargetDir string
	// Whether the torrent is paused.
	Paused bool
	// The time the torrent was added.
	CreatedAt time.Time
	// The time the torrent's persisted state was last modified.
	UpdatedAt time.Time
	// The display name reported in the torrent's metainfo, or empty when the
	// server isn't currently tracking the torrent.
	Name string
	// The total length of the torrent's content, in bytes.
	Length int64
	// How many bytes of the content have been downloaded.
	BytesCompleted int64
	// The number of peers the server is currently connected to.
	ActivePeers int
	// The number of those peers known to be seeders.
	Seeders int
}

func fromAPI(t api.Torrent) Torrent {
	return Torrent{
		InfoHash:       t.InfoHash,
		Magnet:         t.Magnet,
		Label:          t.Label,
		TargetDir:      t.TargetDir,
		Paused:         t.Paused,
		CreatedAt:      t.CreatedAt,
		UpdatedAt:      t.UpdatedAt,
		Name:           t.Name,
		Length:         t.Length,
		BytesCompleted: t.BytesCompleted,
		ActivePeers:    t.ActivePeers,
		Seeders:        t.Seeders,
	}
}

// AddMagnet adds a torrent by magnet URI. The label and targetDir are optional.
func (c *Client) AddMagnet(ctx context.Context, magnet, label, targetDir string) (Torrent, error) {
	response, err := do[api.AddTorrentResponse](ctx, c, http.MethodPost, "/api/v1/torrents", api.AddTorrentRequest{
		Magnet:    magnet,
		Label:     label,
		TargetDir: targetDir,
	})
	if err != nil {
		return Torrent{}, err
	}

	return fromAPI(response.Torrent), nil
}

// List returns every managed torrent.
func (c *Client) List(ctx context.Context) ([]Torrent, error) {
	response, err := do[api.ListTorrentsResponse](ctx, c, http.MethodGet, "/api/v1/torrents", nil)
	if err != nil {
		return nil, err
	}

	torrents := make([]Torrent, len(response.Torrents))
	for i, t := range response.Torrents {
		torrents[i] = fromAPI(t)
	}

	return torrents, nil
}

// Get returns a single managed torrent identified by hash.
func (c *Client) Get(ctx context.Context, hash string) (Torrent, error) {
	response, err := do[api.GetTorrentResponse](ctx, c, http.MethodGet, path.Join("/api/v1/torrents", hash), nil)
	if err != nil {
		return Torrent{}, err
	}

	return fromAPI(response.Torrent), nil
}

// Remove removes a managed torrent identified by hash.
func (c *Client) Remove(ctx context.Context, hash string) error {
	_, err := do[api.RemoveTorrentResponse](ctx, c, http.MethodDelete, path.Join("/api/v1/torrents", hash), nil)
	return err
}

// Pause pauses a managed torrent identified by hash.
func (c *Client) Pause(ctx context.Context, hash string) error {
	_, err := do[api.PauseTorrentResponse](ctx, c, http.MethodPost, path.Join("/api/v1/torrents", hash, "pause"), nil)
	return err
}

// Resume resumes a managed torrent identified by hash.
func (c *Client) Resume(ctx context.Context, hash string) error {
	_, err := do[api.ResumeTorrentResponse](ctx, c, http.MethodPost, path.Join("/api/v1/torrents", hash, "resume"), nil)
	return err
}
