-- Migration 007: kingdoms role cleanup + server heartbeat table
-- Sprint 2: rename "king" → "basileus", drop "advisor"; add server_heartbeats for GameClock.

-- Kingdoms role rename.
UPDATE kingdom_members SET role = 'basileus' WHERE role = 'king';
DELETE FROM kingdom_members WHERE role = 'advisor';

-- server_heartbeats — one row per 10-second beat.
-- Kept small by a rolling retention window (HEARTBEAT_RETENTION, default 168h per
-- DR1); older rows pruned by the background retention job in cmd/server/main.go.
CREATE TABLE server_heartbeats (
    id      BIGSERIAL PRIMARY KEY,
    beat_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_server_heartbeats_beat_at ON server_heartbeats (beat_at DESC);
