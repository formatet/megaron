-- Stable queue membership survives read/status updates. xmin cannot serve as
-- insertion identity because every UPDATE replaces it. Existing letters are
-- backfilled in this transaction; subsequent inserts use their creating TX.
ALTER TABLE messengers ADD COLUMN correspondence_created_xid xid8 NOT NULL DEFAULT pg_current_xact_id();
CREATE INDEX idx_messengers_correspondence_page ON messengers(world_id, sent_at DESC, id DESC);
