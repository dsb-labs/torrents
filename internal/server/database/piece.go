package database

import (
	"context"
	"database/sql"
	"fmt"
	"maps"
	"sync"

	"github.com/anacrolix/torrent/metainfo"
	"github.com/anacrolix/torrent/storage"
)

// The PieceRepository type provides persistence operations for the
// piece-completion state used by the embedded torrent client. It implements
// anacrolix/torrent's storage.PieceCompletion against the shared *sql.DB,
// with an in-memory cache populated on construction so the per-piece Get
// calls anacrolix issues during VerifyDataContext don't each become a SQL
// round-trip.
type PieceRepository struct {
	db    *sql.DB
	mu    sync.RWMutex
	cache map[metainfo.PieceKey]bool
}

// Compile-time guarantee that PieceRepository satisfies the anacrolix
// storage contract, including the optional Persistent reporter.
var (
	_ storage.PieceCompletion             = (*PieceRepository)(nil)
	_ storage.PieceCompletionPersistenter = (*PieceRepository)(nil)
)

// NewPieceRepository returns a PieceRepository backed by db. The cache starts
// empty; callers that want the bulk-load performance benefit for restore must
// invoke Load before handing the repository to anacrolix.
func NewPieceRepository(db *sql.DB) *PieceRepository {
	return &PieceRepository{
		db:    db,
		cache: make(map[metainfo.PieceKey]bool),
	}
}

// Load reads the entire piece_completion table into the in-memory cache.
// It's a one-shot sequential scan run at startup so the per-piece Get calls
// anacrolix issues during VerifyDataContext are served from RAM instead of
// turning into thousands of SQL round-trips per torrent.
func (r *PieceRepository) Load(ctx context.Context) error {
	const q = `SELECT info_hash, piece_index, complete FROM piece_completion`

	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return fmt.Errorf("failed to load piece completion cache: %w", err)
	}
	defer rows.Close()

	loaded := make(map[metainfo.PieceKey]bool)

	for rows.Next() {
		var (
			hexHash  string
			index    int
			complete bool
		)

		if err = rows.Scan(&hexHash, &index, &complete); err != nil {
			return fmt.Errorf("failed to scan piece completion row: %w", err)
		}

		var hash metainfo.Hash
		if err = hash.FromHexString(hexHash); err != nil {
			return fmt.Errorf("failed to parse piece completion info hash %q: %w", hexHash, err)
		}

		loaded[metainfo.PieceKey{InfoHash: hash, Index: index}] = complete
	}

	if err = rows.Err(); err != nil {
		return fmt.Errorf("failed to read piece completion rows: %w", err)
	}

	r.mu.Lock()
	r.cache = loaded
	r.mu.Unlock()

	return nil
}

// Get returns the completion state for the given piece. When no row exists
// the returned Completion has Ok=false, signalling to the engine that the
// piece's state is unknown and should be (re-)hashed against the on-disk data.
func (r *PieceRepository) Get(pk metainfo.PieceKey) (storage.Completion, error) {
	r.mu.RLock()
	complete, ok := r.cache[pk]
	r.mu.RUnlock()

	if !ok {
		return storage.Completion{}, nil
	}

	return storage.Completion{Ok: true, Complete: complete}, nil
}

// Set records the completion state for the given piece. The mutex is held
// across both the DB write and the cache update so two concurrent Sets to
// the same key can't leave the cache disagreeing with the row on disk.
func (r *PieceRepository) Set(pk metainfo.PieceKey, complete bool) error {
	const q = `INSERT OR REPLACE INTO piece_completion (info_hash, piece_index, complete) VALUES (?, ?, ?)`

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, err := r.db.ExecContext(context.Background(), q, pk.InfoHash.HexString(), pk.Index, complete); err != nil {
		return fmt.Errorf("failed to upsert piece completion: %w", err)
	}

	r.cache[pk] = complete

	return nil
}

// Forget drops the cached piece-completion entries for the torrent identified
// by infoHash. The on-disk piece_completion rows are removed by the
// piece_completion_cascade trigger when the torrent row is deleted, so this
// only takes care of the in-memory side. Service.Remove calls it after
// torrents.Delete so a torrent re-added with the same hash starts fresh
// instead of inheriting the previous instance's completion state.
func (r *PieceRepository) Forget(infoHash string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	maps.DeleteFunc(r.cache, func(key metainfo.PieceKey, b bool) bool {
		return key.InfoHash.HexString() == infoHash
	})
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
