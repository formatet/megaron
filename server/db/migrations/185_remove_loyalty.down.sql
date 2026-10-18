-- Restore the empty legacy schema only. Removed loyalty history/projections
-- cannot be reconstructed from a rollback; scheduled work stays retired.
ALTER TABLE settlements
  ADD COLUMN loyalty INT NOT NULL DEFAULT 2 CHECK (loyalty BETWEEN 1 AND 4),
  ADD COLUMN loyalty_points DOUBLE PRECISION NOT NULL DEFAULT 37,
  ADD COLUMN loyalty_trend TEXT NOT NULL DEFAULT 'stable';
CREATE TABLE loyalty_events (
  id BIGSERIAL PRIMARY KEY,
  settlement_id UUID NOT NULL REFERENCES settlements(id) ON DELETE CASCADE,
  world_id UUID NOT NULL,
  event_type TEXT NOT NULL,
  loyalty_delta INT NOT NULL DEFAULT 0,
  reason TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_loyalty_events_settlement ON loyalty_events (settlement_id, created_at);
