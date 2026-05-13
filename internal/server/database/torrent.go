package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

var (
	// ErrTorrentNotFound is returned when no torrent exists with the requested info hash.
	ErrTorrentNotFound = errors.New("torrent not found")
	// ErrTorrentAlreadyExists is returned when a torrent with the same info hash is already present.
	ErrTorrentAlreadyExists = errors.New("torrent already exists")
)

type (
	// The Torrent type represents a torrent managed by the server.
	Torrent struct {
		// The torrent's BitTorrent info hash (40-character hex string).
		InfoHash string
		// The original magnet URI used to add the torrent.
		Magnet string
		// A human-supplied label, or the empty string if unset.
		Label string
		// The filesystem directory the torrent's content is written into.
		TargetDir string
		// Whether the torrent is currently paused.
		IsPaused bool
		// The time the torrent was added.
		CreatedAt time.Time
		// The time the torrent's row was last modified.
		UpdatedAt time.Time
	}

	// The TorrentRepository type provides persistence operations for the torrent domain.
	TorrentRepository struct {
		db *sql.DB
	}
)

// NewTorrentRepository returns a TorrentRepository backed by the given pool.
func NewTorrentRepository(db *sql.DB) *TorrentRepository {
	return &TorrentRepository{db: db}
}

// Create inserts t. IsPaused, CreatedAt, and UpdatedAt on the input are ignored:
// new torrents start unpaused and the repository assigns the timestamps.
// Returns ErrTorrentAlreadyExists when a torrent with the same info hash is already present.
func (r *TorrentRepository) Create(ctx context.Context, t Torrent) error {
	const q = `
		INSERT INTO torrent (info_hash, magnet, label, target_dir, is_paused, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`

	now := formatTime(time.Now())

	_, err := r.db.ExecContext(ctx, q, t.InfoHash, t.Magnet, t.Label, t.TargetDir, false, now, now)
	switch {
	case IsUniqueError(err):
		return ErrTorrentAlreadyExists
	case err != nil:
		return fmt.Errorf("failed to insert torrent: %w", err)
	}

	return nil
}

// Get returns the torrent identified by the given info hash. Returns
// ErrTorrentNotFound when no such torrent exists.
func (r *TorrentRepository) Get(ctx context.Context, infoHash string) (Torrent, error) {
	const q = `
		SELECT info_hash, magnet, label, target_dir, is_paused, created_at, updated_at
		FROM torrent
		WHERE info_hash = ?
	`

	var (
		t                    Torrent
		createdAt, updatedAt string
	)

	err := r.db.QueryRowContext(ctx, q, infoHash).Scan(
		&t.InfoHash,
		&t.Magnet,
		&t.Label,
		&t.TargetDir,
		&t.IsPaused,
		&createdAt,
		&updatedAt,
	)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return Torrent{}, ErrTorrentNotFound
	case err != nil:
		return Torrent{}, fmt.Errorf("failed to load torrent: %w", err)
	}

	t.CreatedAt, t.UpdatedAt, err = parseTimestamps(createdAt, updatedAt)
	if err != nil {
		return Torrent{}, err
	}

	return t, nil
}

// List returns every torrent in the repository, ordered by creation time ascending.
func (r *TorrentRepository) List(ctx context.Context) ([]Torrent, error) {
	const q = `
		SELECT info_hash, magnet, label, target_dir, is_paused, created_at, updated_at
		FROM torrent
		ORDER BY created_at ASC
	`

	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("failed to query torrents: %w", err)
	}
	defer rows.Close()

	var torrents []Torrent
	for rows.Next() {
		var (
			t                    Torrent
			createdAt, updatedAt string
		)

		if err = rows.Scan(
			&t.InfoHash,
			&t.Magnet,
			&t.Label,
			&t.TargetDir,
			&t.IsPaused,
			&createdAt,
			&updatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan torrent: %w", err)
		}

		t.CreatedAt, t.UpdatedAt, err = parseTimestamps(createdAt, updatedAt)
		if err != nil {
			return nil, err
		}

		torrents = append(torrents, t)
	}

	return torrents, rows.Err()
}

// Delete removes the torrent identified by the given info hash. Returns
// ErrTorrentNotFound when no such torrent exists.
func (r *TorrentRepository) Delete(ctx context.Context, infoHash string) error {
	const q = `DELETE FROM torrent WHERE info_hash = ?`

	result, err := r.db.ExecContext(ctx, q, infoHash)
	if err != nil {
		return fmt.Errorf("failed to delete torrent: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to read affected rows: %w", err)
	}

	if rows == 0 {
		return ErrTorrentNotFound
	}

	return nil
}

// SetPaused updates the paused state of the torrent identified by the given info hash.
// Returns ErrTorrentNotFound when no such torrent exists.
func (r *TorrentRepository) SetPaused(ctx context.Context, infoHash string, paused bool) error {
	const q = `
		UPDATE torrent
		SET is_paused = ?, updated_at = ?
		WHERE info_hash = ?
	`

	result, err := r.db.ExecContext(ctx, q, paused, formatTime(time.Now()), infoHash)
	if err != nil {
		return fmt.Errorf("failed to update torrent: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to read affected rows: %w", err)
	}

	if rows == 0 {
		return ErrTorrentNotFound
	}

	return nil
}

