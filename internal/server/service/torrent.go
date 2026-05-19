// Package service provides the domain orchestration layer for the torrents server.
package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/dsb-labs/torrents/internal/server/database"
	"github.com/dsb-labs/torrents/internal/server/torrent"
)

var (
	// ErrTorrentNotFound is returned when the requested torrent does not exist.
	ErrTorrentNotFound = errors.New("torrent not found")
	// ErrTorrentAlreadyExists is returned when adding a torrent that is already present.
	ErrTorrentAlreadyExists = errors.New("torrent already exists")
)

type (
	// The TorrentEngine interface describes the torrent-runtime operations the service uses.
	TorrentEngine interface {
		// AddMagnet should add a torrent identified by the given magnet URI.
		AddMagnet(ctx context.Context, uri string) (torrent.InfoHash, error)
		// AddFile should add a torrent from a .torrent metainfo file read from r.
		AddFile(ctx context.Context, r io.Reader) (torrent.InfoHash, error)
		// Remove should stop tracking the torrent identified by hash.
		Remove(ctx context.Context, hash torrent.InfoHash) error
		// Snapshot should return the current live state of the torrent identified by hash.
		Snapshot(hash torrent.InfoHash) (torrent.Progress, error)
		// Files should return the per-file progress for the torrent identified by hash.
		Files(hash torrent.InfoHash) ([]torrent.FileProgress, error)
	}

	// The PieceRepository interface describes the piece-completion cache
	// operations the service needs to keep in sync with torrent lifecycle events.
	PieceRepository interface {
		// Forget should drop any cached piece-completion entries for the
		// torrent identified by infoHash.
		Forget(infoHash string)
	}

	// The TorrentRepository interface describes the persistence operations the service uses.
	TorrentRepository interface {
		// Create should insert t into the repository.
		Create(ctx context.Context, t database.Torrent) error
		// Get should return the torrent identified by infoHash.
		Get(ctx context.Context, infoHash string) (database.Torrent, error)
		// List should return every torrent in the repository.
		List(ctx context.Context) ([]database.Torrent, error)
		// Delete should remove the torrent identified by infoHash.
		Delete(ctx context.Context, infoHash string) error
		// SetMetadata should persist the display name and total length for the
		// torrent identified by infoHash.
		SetMetadata(ctx context.Context, infoHash, name string, length int64) error
		// SetBytesCompleted should persist the bytes-completed counter for the
		// torrent identified by infoHash.
		SetBytesCompleted(ctx context.Context, infoHash string, bytesCompleted int64) error
		// SetPaused should update the paused state of the torrent identified by infoHash.
		SetPaused(ctx context.Context, infoHash string, paused bool) error
	}

	// The Torrent type represents a managed torrent, combining persisted metadata
	// with the engine's live transfer state.
	Torrent struct {
		// The torrent's info hash.
		InfoHash string
		// The magnet URI the torrent was added with.
		Magnet string
		// A human-supplied label, or the empty string if unset.
		Label string
		// The filesystem directory the torrent's content is written into.
		TargetDir string
		// Whether the torrent is paused.
		Paused bool
		// The time the torrent was added.
		CreatedAt time.Time
		// The time the torrent's persisted state was last modified.
		UpdatedAt time.Time
		// The display name reported in the torrent's metainfo, or empty when
		// the engine isn't currently tracking the torrent.
		Name string
		// The total length of the torrent's content, in bytes.
		Length int64
		// How many bytes of the content have been downloaded.
		BytesCompleted int64
		// The number of peers the engine is currently connected to.
		ActivePeers int
		// The number of those peers known to be seeders.
		Seeders int
	}

	// The File type represents a single file within a managed torrent.
	File struct {
		// The file's path within the torrent.
		Path string
		// The total length of the file, in bytes.
		Length int64
		// How many bytes of the file have been downloaded.
		BytesCompleted int64
	}

	// The AddOptions type carries the metadata supplied when adding a torrent.
	AddOptions struct {
		// An optional human-supplied label.
		Label string
		// The filesystem directory the torrent's content should be written into.
		TargetDir string
	}

	// The TorrentService type orchestrates the torrent engine and persistence layer.
	TorrentService struct {
		logger   *slog.Logger
		engine   TorrentEngine
		torrents TorrentRepository
		pieces   PieceRepository
	}
)

// NewTorrentService returns a TorrentService that operates on the given engine and repositories.
func NewTorrentService(logger *slog.Logger, engine TorrentEngine, torrents TorrentRepository, pieces PieceRepository) *TorrentService {
	return &TorrentService{
		logger:   logger.With("component", "service"),
		engine:   engine,
		torrents: torrents,
		pieces:   pieces,
	}
}

// AddMagnet adds a torrent identified by the given magnet URI and persists it.
// Returns ErrTorrentAlreadyExists when the torrent is already managed.
func (s *TorrentService) AddMagnet(ctx context.Context, uri string, opts AddOptions) (Torrent, error) {
	hash, err := s.engine.AddMagnet(ctx, uri)
	if err != nil {
		return Torrent{}, fmt.Errorf("failed to add magnet: %w", err)
	}

	return s.persist(ctx, hash, uri, opts)
}

// AddFile adds a torrent from a .torrent metainfo file read from r and persists it.
// Returns ErrTorrentAlreadyExists when the torrent is already managed.
func (s *TorrentService) AddFile(ctx context.Context, r io.Reader, opts AddOptions) (Torrent, error) {
	hash, err := s.engine.AddFile(ctx, r)
	if err != nil {
		return Torrent{}, fmt.Errorf("failed to add torrent file: %w", err)
	}

	return s.persist(ctx, hash, fmt.Sprintf("magnet:?xt=urn:btih:%s", hash), opts)
}

// Get returns the managed torrent identified by infoHash. Returns ErrTorrentNotFound
// when no such torrent is managed.
func (s *TorrentService) Get(ctx context.Context, infoHash string) (Torrent, error) {
	row, err := s.torrents.Get(ctx, infoHash)
	switch {
	case errors.Is(err, database.ErrTorrentNotFound):
		return Torrent{}, ErrTorrentNotFound
	case err != nil:
		return Torrent{}, fmt.Errorf("failed to load torrent: %w", err)
	}

	return s.hydrate(row), nil
}

// Files returns the per-file progress for the torrent identified by infoHash.
// Returns nil with no error when the torrent is paused and file data is unavailable.
// Returns ErrTorrentNotFound when no such torrent is managed.
func (s *TorrentService) Files(ctx context.Context, infoHash string) ([]File, error) {
	_, err := s.torrents.Get(ctx, infoHash)
	switch {
	case errors.Is(err, database.ErrTorrentNotFound):
		return nil, ErrTorrentNotFound
	case err != nil:
		return nil, fmt.Errorf("failed to load torrent: %w", err)
	}

	fp, err := s.engine.Files(torrent.InfoHash(infoHash))
	if err != nil {
		return nil, nil
	}

	files := make([]File, len(fp))
	for i, f := range fp {
		files[i] = File{
			Path:           f.Path,
			Length:         f.Length,
			BytesCompleted: f.BytesCompleted,
		}
	}

	return files, nil
}

// List returns every managed torrent, with the engine's live state where available.
func (s *TorrentService) List(ctx context.Context) ([]Torrent, error) {
	rows, err := s.torrents.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list torrents: %w", err)
	}

	torrents := make([]Torrent, len(rows))
	for i, row := range rows {
		torrents[i] = s.hydrate(row)
	}

	return torrents, nil
}

// Remove removes the torrent identified by infoHash from both the engine and the
// repository. Returns ErrTorrentNotFound when no such torrent is managed.
func (s *TorrentService) Remove(ctx context.Context, infoHash string) error {
	err := s.engine.Remove(ctx, torrent.InfoHash(infoHash))
	switch {
	case errors.Is(err, torrent.ErrNotFound):
		// Engine doesn't know about it; fall through to delete the row.
	case err != nil:
		return fmt.Errorf("failed to remove torrent from engine: %w", err)
	}

	err = s.torrents.Delete(ctx, infoHash)
	switch {
	case errors.Is(err, database.ErrTorrentNotFound):
		return ErrTorrentNotFound
	case err != nil:
		return fmt.Errorf("failed to delete torrent: %w", err)
	}

	s.pieces.Forget(infoHash)

	return nil
}

// Pause stops the torrent identified by infoHash by removing it from the engine
// and recording the paused state in the repository. The engine's current
// bytes-completed counter is captured first so the UI can keep showing
// progress while the torrent is paused. Returns ErrTorrentNotFound when no
// such torrent is managed.
func (s *TorrentService) Pause(ctx context.Context, infoHash string) error {
	s.persistBytesCompleted(ctx, torrent.InfoHash(infoHash))

	err := s.engine.Remove(ctx, torrent.InfoHash(infoHash))
	switch {
	case errors.Is(err, torrent.ErrNotFound):
		// Engine wasn't tracking it (already paused or never added); fall through.
	case err != nil:
		return fmt.Errorf("failed to remove torrent from engine: %w", err)
	}

	err = s.torrents.SetPaused(ctx, infoHash, true)
	switch {
	case errors.Is(err, database.ErrTorrentNotFound):
		return ErrTorrentNotFound
	case err != nil:
		return fmt.Errorf("failed to persist torrent state: %w", err)
	}

	return nil
}

// Resume re-adds the torrent identified by infoHash to the engine using its
// persisted magnet URI and records the unpaused state. Returns ErrTorrentNotFound
// when no such torrent is managed.
//
// The paused flag is flipped to false BEFORE engine.AddMagnet runs so the
// row reflects the new state immediately; engine.AddMagnet can block for
// seconds while VerifyDataContext re-hashes existing on-disk data, and the
// UI polls every 2s, so an early flip lets the user see the state change
// without waiting on verification. If engine.AddMagnet fails, the flag is
// rolled back to true.
func (s *TorrentService) Resume(ctx context.Context, infoHash string) error {
	row, err := s.torrents.Get(ctx, infoHash)
	switch {
	case errors.Is(err, database.ErrTorrentNotFound):
		return ErrTorrentNotFound
	case err != nil:
		return fmt.Errorf("failed to load torrent: %w", err)
	}

	err = s.torrents.SetPaused(ctx, infoHash, false)
	switch {
	case errors.Is(err, database.ErrTorrentNotFound):
		return ErrTorrentNotFound
	case err != nil:
		return fmt.Errorf("failed to persist torrent state: %w", err)
	}

	hash, err := s.engine.AddMagnet(ctx, row.Magnet)
	if err != nil {
		if rollbackErr := s.torrents.SetPaused(ctx, infoHash, true); rollbackErr != nil {
			s.logger.With("info_hash", infoHash, "error", rollbackErr).Error("failed to roll back pause state after resume failure")
		}
		return fmt.Errorf("failed to re-add torrent to engine: %w", err)
	}

	s.persistMetadata(ctx, hash)

	return nil
}

// Restore re-adds every non-paused persisted torrent to the engine. Intended to
// be called once during server startup. Paused torrents stay DB-only until the
// user resumes them. Individual torrents that fail to restore are logged and
// skipped; Restore returns an error only when the repository read itself fails.
func (s *TorrentService) Restore(ctx context.Context) error {
	rows, err := s.torrents.List(ctx)
	if err != nil {
		return fmt.Errorf("failed to list torrents: %w", err)
	}

	group, ctx := errgroup.WithContext(ctx)
	for _, row := range rows {
		if row.Paused {
			continue
		}

		s.logger.With("info_hash", row.InfoHash).Debug("restoring torrent")

		group.Go(func() error {
			hash, err := s.engine.AddMagnet(ctx, row.Magnet)
			if err != nil {
				s.logger.With("info_hash", row.InfoHash, "error", err).Error("failed to restore torrent to engine")
				return nil
			}

			s.persistMetadata(ctx, hash)

			s.logger.With("info_hash", row.InfoHash).Debug("torrent restored")
			return ctx.Err()
		})
	}

	return group.Wait()
}

func (s *TorrentService) persist(ctx context.Context, hash torrent.InfoHash, magnet string, opts AddOptions) (Torrent, error) {
	err := s.torrents.Create(ctx, database.Torrent{
		InfoHash:  string(hash),
		Magnet:    magnet,
		Label:     opts.Label,
		TargetDir: opts.TargetDir,
	})
	switch {
	case errors.Is(err, database.ErrTorrentAlreadyExists):
		return Torrent{}, ErrTorrentAlreadyExists
	case err != nil:
		return Torrent{}, fmt.Errorf("failed to persist torrent: %w", err)
	}

	s.persistMetadata(ctx, hash)

	return s.Get(ctx, string(hash))
}

// persistMetadata captures the engine's current name and length for the
// torrent identified by hash and writes them to the repository so the UI can
// still render them after the torrent is paused (and dropped from the engine).
// Failures are logged and swallowed: the caller's primary operation succeeded
// and a missing-name fallback is recoverable on the next add/resume.
func (s *TorrentService) persistMetadata(ctx context.Context, hash torrent.InfoHash) {
	progress, err := s.engine.Snapshot(hash)
	if err != nil || progress.Name == "" {
		return
	}

	if err := s.torrents.SetMetadata(ctx, string(hash), progress.Name, progress.Length); err != nil {
		s.logger.With("info_hash", hash, "error", err).Error("failed to persist torrent metadata")
	}
}

// persistBytesCompleted captures the engine's current bytes-completed counter
// for the torrent identified by hash and writes it to the repository so the
// UI keeps showing progress after the torrent is paused. Failures are logged
// and swallowed: the caller's primary operation is the state change, and a
// stale counter is preferable to aborting the pause.
func (s *TorrentService) persistBytesCompleted(ctx context.Context, hash torrent.InfoHash) {
	progress, err := s.engine.Snapshot(hash)
	if err != nil {
		return
	}

	if err := s.torrents.SetBytesCompleted(ctx, string(hash), progress.BytesCompleted); err != nil {
		s.logger.With("info_hash", hash, "error", err).Error("failed to persist torrent bytes completed")
	}
}

func (s *TorrentService) hydrate(row database.Torrent) Torrent {
	t := Torrent{
		InfoHash:       row.InfoHash,
		Magnet:         row.Magnet,
		Label:          row.Label,
		TargetDir:      row.TargetDir,
		Paused:         row.Paused,
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
		Name:           row.Name,
		Length:         row.Length,
		BytesCompleted: row.BytesCompleted,
	}

	progress, err := s.engine.Snapshot(torrent.InfoHash(row.InfoHash))
	if err != nil {
		return t
	}

	if progress.Name != "" {
		t.Name = progress.Name
	}
	if progress.Length > 0 {
		t.Length = progress.Length
	}
	t.BytesCompleted = progress.BytesCompleted
	t.ActivePeers = progress.ActivePeers
	t.Seeders = progress.Seeders

	return t
}
