-- First observed authenticated contact, never reconstructed from login history.
ALTER TABLE players ADD COLUMN last_seen_at TIMESTAMPTZ;
