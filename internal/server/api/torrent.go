package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	validation "github.com/go-ozzo/ozzo-validation/v4"

	"github.com/dsb-labs/torrents/internal/server/service"
)

type (
	// The TorrentService interface describes the service operations the
	// TorrentAPI exposes over HTTP.
	TorrentService interface {
		// AddMagnet should add a torrent identified by the given magnet URI.
		AddMagnet(ctx context.Context, uri string, opts service.AddOptions) (service.Torrent, error)
		// Get should return the torrent identified by infoHash.
		Get(ctx context.Context, infoHash string) (service.Torrent, error)
		// List should return every managed torrent.
		List(ctx context.Context) ([]service.Torrent, error)
		// Remove should remove the torrent identified by infoHash.
		Remove(ctx context.Context, infoHash string) error
		// Pause should pause the torrent identified by infoHash.
		Pause(ctx context.Context, infoHash string) error
		// Resume should resume the torrent identified by infoHash.
		Resume(ctx context.Context, infoHash string) error
	}

	// The TorrentAPI type exposes HTTP endpoints for managing torrents.
	TorrentAPI struct {
		torrents TorrentService
	}
)

// NewTorrentAPI returns a new TorrentAPI backed by the given service.
func NewTorrentAPI(torrents TorrentService) *TorrentAPI {
	return &TorrentAPI{torrents: torrents}
}

// Register the HTTP endpoints onto the given http.ServeMux.
func (api *TorrentAPI) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/torrents", api.Add)
	mux.HandleFunc("GET /api/v1/torrents", api.List)
	mux.HandleFunc("GET /api/v1/torrents/{hash}", api.Get)
	mux.HandleFunc("DELETE /api/v1/torrents/{hash}", api.Remove)
	mux.HandleFunc("POST /api/v1/torrents/{hash}/pause", api.Pause)
	mux.HandleFunc("POST /api/v1/torrents/{hash}/resume", api.Resume)
}

type (
	// The AddTorrentRequest type is the JSON body for POST /api/v1/torrents.
	AddTorrentRequest struct {
		// The magnet URI of the torrent to add.
		Magnet string `json:"magnet"`
		// An optional human-supplied label.
		Label string `json:"label"`
		// The filesystem directory the torrent's content should be written into.
		TargetDir string `json:"targetDir"`
	}

	// The AddTorrentResponse type is the JSON body returned by POST /api/v1/torrents.
	AddTorrentResponse struct {
		// The added torrent.
		Torrent Torrent `json:"torrent"`
	}

	// The Torrent type is the JSON shape of a managed torrent.
	Torrent struct {
		// The torrent's info hash.
		InfoHash string `json:"infoHash"`
		// The magnet URI the torrent was added with.
		Magnet string `json:"magnet"`
		// A human-supplied label, or empty when unset.
		Label string `json:"label"`
		// The filesystem directory the torrent's content is written into.
		TargetDir string `json:"targetDir"`
		// Whether the torrent is paused.
		Paused bool `json:"paused"`
		// The time the torrent was added.
		CreatedAt time.Time `json:"createdAt"`
		// The time the torrent's persisted state was last modified.
		UpdatedAt time.Time `json:"updatedAt"`
		// The display name reported in the torrent's metainfo, or empty when
		// the engine isn't currently tracking the torrent.
		Name string `json:"name,omitempty"`
		// The total length of the torrent's content, in bytes.
		Length int64 `json:"length,omitempty"`
		// How many bytes of the content have been downloaded.
		BytesCompleted int64 `json:"bytesCompleted,omitempty"`
		// The number of peers the engine is currently connected to.
		ActivePeers int `json:"activePeers,omitempty"`
		// The number of those peers known to be seeders.
		Seeders int `json:"seeders,omitempty"`
	}
)

// Validate the request fields.
func (r AddTorrentRequest) Validate() error {
	return validation.ValidateStruct(&r,
		validation.Field(&r.Magnet, validation.Required),
	)
}

func newTorrent(t service.Torrent) Torrent {
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

// Add adds a torrent by magnet URI.
func (api *TorrentAPI) Add(w http.ResponseWriter, r *http.Request) {
	request, err := decode[AddTorrentRequest](r.Body)
	if err != nil {
		writeErrorf(w, http.StatusBadRequest, "%v", err)
		return
	}

	t, err := api.torrents.AddMagnet(r.Context(), request.Magnet, service.AddOptions{
		Label:     request.Label,
		TargetDir: request.TargetDir,
	})
	switch {
	case errors.Is(err, service.ErrTorrentAlreadyExists):
		writeErrorf(w, http.StatusConflict, "torrent already exists")
		return
	case err != nil:
		writeErrorf(w, http.StatusInternalServerError, "failed to add torrent: %v", err)
		return
	}

	writeJSON(w, http.StatusCreated, AddTorrentResponse{Torrent: newTorrent(t)})
}

type (
	// The ListTorrentsResponse type is the JSON body returned by GET /api/v1/torrents.
	ListTorrentsResponse struct {
		// The managed torrents.
		Torrents []Torrent `json:"torrents"`
	}
)

// List returns every managed torrent.
func (api *TorrentAPI) List(w http.ResponseWriter, r *http.Request) {
	torrents, err := api.torrents.List(r.Context())
	if err != nil {
		writeErrorf(w, http.StatusInternalServerError, "failed to list torrents: %v", err)
		return
	}

	response := ListTorrentsResponse{Torrents: make([]Torrent, len(torrents))}
	for i, t := range torrents {
		response.Torrents[i] = newTorrent(t)
	}

	writeJSON(w, http.StatusOK, response)
}

type (
	// The GetTorrentResponse type is the JSON body returned by GET /api/v1/torrents/{hash}.
	GetTorrentResponse struct {
		// The requested torrent.
		Torrent Torrent `json:"torrent"`
	}
)

// Get returns a single managed torrent.
func (api *TorrentAPI) Get(w http.ResponseWriter, r *http.Request) {
	t, err := api.torrents.Get(r.Context(), r.PathValue("hash"))
	switch {
	case errors.Is(err, service.ErrTorrentNotFound):
		writeErrorf(w, http.StatusNotFound, "torrent %q does not exist", r.PathValue("hash"))
		return
	case err != nil:
		writeErrorf(w, http.StatusInternalServerError, "failed to load torrent: %v", err)
		return
	}

	writeJSON(w, http.StatusOK, GetTorrentResponse{Torrent: newTorrent(t)})
}

type (
	// The RemoveTorrentResponse type is the JSON body returned by DELETE /api/v1/torrents/{hash}.
	RemoveTorrentResponse struct{}
)

// Remove removes a managed torrent.
func (api *TorrentAPI) Remove(w http.ResponseWriter, r *http.Request) {
	err := api.torrents.Remove(r.Context(), r.PathValue("hash"))
	switch {
	case errors.Is(err, service.ErrTorrentNotFound):
		writeErrorf(w, http.StatusNotFound, "torrent %q does not exist", r.PathValue("hash"))
		return
	case err != nil:
		writeErrorf(w, http.StatusInternalServerError, "failed to remove torrent: %v", err)
		return
	}

	writeJSON(w, http.StatusOK, RemoveTorrentResponse{})
}

type (
	// The PauseTorrentResponse type is the JSON body returned by POST /api/v1/torrents/{hash}/pause.
	PauseTorrentResponse struct{}
)

// Pause pauses a managed torrent.
func (api *TorrentAPI) Pause(w http.ResponseWriter, r *http.Request) {
	err := api.torrents.Pause(r.Context(), r.PathValue("hash"))
	switch {
	case errors.Is(err, service.ErrTorrentNotFound):
		writeErrorf(w, http.StatusNotFound, "torrent %q does not exist", r.PathValue("hash"))
		return
	case err != nil:
		writeErrorf(w, http.StatusInternalServerError, "failed to pause torrent: %v", err)
		return
	}

	writeJSON(w, http.StatusOK, PauseTorrentResponse{})
}

type (
	// The ResumeTorrentResponse type is the JSON body returned by POST /api/v1/torrents/{hash}/resume.
	ResumeTorrentResponse struct{}
)

// Resume resumes a managed torrent.
func (api *TorrentAPI) Resume(w http.ResponseWriter, r *http.Request) {
	err := api.torrents.Resume(r.Context(), r.PathValue("hash"))
	switch {
	case errors.Is(err, service.ErrTorrentNotFound):
		writeErrorf(w, http.StatusNotFound, "torrent %q does not exist", r.PathValue("hash"))
		return
	case err != nil:
		writeErrorf(w, http.StatusInternalServerError, "failed to resume torrent: %v", err)
		return
	}

	writeJSON(w, http.StatusOK, ResumeTorrentResponse{})
}
