CREATE TABLE IF NOT EXISTS torrent (
    info_hash  TEXT NOT NULL PRIMARY KEY,
    magnet     TEXT NOT NULL,
    label      TEXT NOT NULL DEFAULT '',
    target_dir TEXT NOT NULL,
    is_paused  INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_torrent_created_at ON torrent(created_at);
