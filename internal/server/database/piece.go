package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/anacrolix/torrent/metainfo"
	"github.com/anacrolix/torrent/storage"
)

// The PieceRepository type provides persistence operations for the
// piece-completion state used by the embedded torrent client. It implements
// anacrolix/torrent's storage.PieceCompletion against the shared *sql.DB.
type PieceRepository struct {
	db *sql.DB
}

// Compile-time guarantee that PieceRepository satisfies the anacrolix
// storage contract, including the optional Persistent reporter.
var (
	_ storage.PieceCompletion             = (*PieceRepository)(nil)
	_ storage.PieceCompletionPersistenter = (*PieceRepository)(nil)
)

// NewPieceRepository returns a PieceRepository backed by the given pool.
func NewPieceRepository(db *sql.DB) *PieceRepository {
	return &PieceRepository{db: db}
}

// Get returns the completion state for the given piece. When no row exists
// the returned Completion has Ok=false, signalling to the engine that the
// piece's state is unknown and should be (re-)hashed against the on-disk data.
func (r *PieceRepository) Get(pk metainfo.PieceKey) (storage.Completion, error) {
	const q = `SELECT complete FROM piece_completion WHERE info_hash = ? AND piece_index = ?`

	var complete bool

	err := r.db.QueryRowContext(context.Background(), q, pk.InfoHash.HexString(), pk.Index).Scan(&complete)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return storage.Completion{}, nil
	case err != nil:
		return storage.Completion{}, fmt.Errorf("failed to query piece completion: %w", err)
	}

	return storage.Completion{Ok: true, Complete: complete}, nil
}

// Set records the completion state for the given piece.
func (r *PieceRepository) Set(pk metainfo.PieceKey, complete bool) error {
	const q = `INSERT OR REPLACE INTO piece_completion (info_hash, piece_index, complete) VALUES (?, ?, ?)`

	if _, err := r.db.ExecContext(context.Background(), q, pk.InfoHash.HexString(), pk.Index, complete); err != nil {
		return fmt.Errorf("failed to upsert piece completion: %w", err)
	}

	return nil
}

// Close is a no-op: the underlying *sql.DB is owned by the server and
// closed at application exit, after the torrent client has finished
// shutting down.
func (r *PieceRepository) Close() error {
	return nil
}

// Persistent reports that piece-completion state is retained across torrent
// client restarts.
func (r *PieceRepository) Persistent() bool {
	return true
}
