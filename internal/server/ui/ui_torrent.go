package ui

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	validation "github.com/go-ozzo/ozzo-validation/v4"

	"github.com/dsb-labs/torrents/internal/server/service"
	"github.com/dsb-labs/torrents/internal/server/ui/component"
	torrentview "github.com/dsb-labs/torrents/internal/server/ui/view/torrent"
)

type (
	// The TorrentService interface describes the service operations the
	// TorrentHandler exposes to the browser.
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

	// The TorrentHandler type renders the torrent management pages and HTMX fragments.
	TorrentHandler struct {
		torrents TorrentService
	}
)

// NewTorrentHandler returns a TorrentHandler backed by the given service.
func NewTorrentHandler(torrents TorrentService) *TorrentHandler {
	return &TorrentHandler{torrents: torrents}
}

// Register the UI endpoints onto the given http.ServeMux.
func (h *TorrentHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc(http.MethodGet+" /{$}", h.List)
	mux.HandleFunc(http.MethodGet+" /torrents/new", h.New)
	mux.HandleFunc(http.MethodPost+" /torrents", h.Add)
	mux.HandleFunc(http.MethodGet+" /ui/torrents", h.Table)
	mux.HandleFunc(http.MethodPost+" /ui/torrents/{hash}/pause", h.Pause)
	mux.HandleFunc(http.MethodPost+" /ui/torrents/{hash}/resume", h.Resume)
	mux.HandleFunc(http.MethodDelete+" /ui/torrents/{hash}", h.Delete)
}

// List renders the full torrent management page.
func (h *TorrentHandler) List(w http.ResponseWriter, r *http.Request) {
	torrents, err := h.torrents.List(r.Context())
	if err != nil {
		http.Error(w, "failed to list torrents", http.StatusInternalServerError)
		return
	}

	render(r.Context(), w, http.StatusOK, torrentview.List, torrentview.ListViewModel{Torrents: torrents})
}

// New renders the dedicated Add Torrent page.
func (h *TorrentHandler) New(w http.ResponseWriter, r *http.Request) {
	render(r.Context(), w, http.StatusOK, torrentview.New, torrentview.NewViewModel{})
}

// Table renders the torrent table fragment (the HTMX polling target).
func (h *TorrentHandler) Table(w http.ResponseWriter, r *http.Request) {
	torrents, err := h.torrents.List(r.Context())
	if err != nil {
		http.Error(w, "failed to list torrents", http.StatusInternalServerError)
		return
	}

	render(r.Context(), w, http.StatusOK, component.TorrentTable, component.TorrentTableProps{Torrents: torrents})
}

type (
	// The addTorrentForm type holds the form fields submitted to add a torrent.
	addTorrentForm struct {
		Magnet    string `form:"magnet"`
		Label     string `form:"label"`
		TargetDir string `form:"target_dir"`
	}
)

// Validate the form fields.
func (f addTorrentForm) Validate() error {
	return validation.ValidateStruct(&f,
		validation.Field(&f.Magnet, validation.Required),
	)
}

// Add handles the Add Torrent page's form submission. On success it redirects
// the browser back to the list page; on validation or service failure it
// re-renders the new view with the submitted values and an error message.
func (h *TorrentHandler) Add(w http.ResponseWriter, r *http.Request) {
	form, err := decode[addTorrentForm](r)
	if err != nil {
		h.renderNewError(w, r, form, err.Error())
		return
	}

	_, err = h.torrents.AddMagnet(r.Context(), form.Magnet, service.AddOptions{
		Label:     form.Label,
		TargetDir: form.TargetDir,
	})
	switch {
	case errors.Is(err, service.ErrTorrentAlreadyExists):
		h.renderNewError(w, r, form, "torrent already exists")
		return
	case err != nil:
		h.renderNewError(w, r, form, fmt.Sprintf("failed to add torrent: %v", err))
		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// Pause pauses a torrent and renders the updated row.
func (h *TorrentHandler) Pause(w http.ResponseWriter, r *http.Request) {
	h.toggle(w, r, h.torrents.Pause)
}

// Resume resumes a torrent and renders the updated row.
func (h *TorrentHandler) Resume(w http.ResponseWriter, r *http.Request) {
	h.toggle(w, r, h.torrents.Resume)
}

// Delete removes a torrent and returns an empty body so HTMX removes the row.
func (h *TorrentHandler) Delete(w http.ResponseWriter, r *http.Request) {
	hash := r.PathValue("hash")
	if err := h.torrents.Remove(r.Context(), hash); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (h *TorrentHandler) toggle(w http.ResponseWriter, r *http.Request, action func(context.Context, string) error) {
	hash := r.PathValue("hash")
	if err := action(r.Context(), hash); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	t, err := h.torrents.Get(r.Context(), hash)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	render(r.Context(), w, http.StatusOK, component.TorrentRow, component.TorrentRowProps{Torrent: t})
}

func (h *TorrentHandler) renderNewError(w http.ResponseWriter, r *http.Request, form addTorrentForm, message string) {
	render(r.Context(), w, http.StatusBadRequest, torrentview.New, torrentview.NewViewModel{
		Magnet:    form.Magnet,
		Label:     form.Label,
		TargetDir: form.TargetDir,
		Error:     message,
	})
}
