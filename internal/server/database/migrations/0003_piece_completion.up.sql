CREATE TABLE piece_completion (
    info_hash   TEXT NOT NULL,
    piece_index INTEGER NOT NULL,
    complete    INTEGER NOT NULL,
    PRIMARY KEY (info_hash, piece_index)
) WITHOUT ROWID;

-- The trigger cascades a torrent's piece completion rows when the torrent
-- itself is deleted. A FOREIGN KEY ... ON DELETE CASCADE would express this
-- declaratively, but anacrolix/torrent writes piece_completion rows during
-- engine.AddMagnet (inside VerifyDataContext) before our service inserts the
-- torrent row, so a FK constraint would reject those writes. A trigger
-- enforces the same cleanup invariant without constraining the insert order.
CREATE TRIGGER piece_completion_cascade
AFTER DELETE ON torrent
FOR EACH ROW
BEGIN
    DELETE FROM piece_completion WHERE info_hash = OLD.info_hash;
END;
