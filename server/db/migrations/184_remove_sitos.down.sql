-- Restore the old empty storage/claim schema only. Returned food stays in the
-- city; repartitioning it would require historical snapshots and would risk loss.
CREATE TABLE settlement_granary (
 settlement_id uuid NOT NULL REFERENCES settlements(id) ON DELETE CASCADE,
 good_key text NOT NULL REFERENCES goods(key),
 amount float8 NOT NULL DEFAULT 0 CHECK(amount >= 0),
 PRIMARY KEY(settlement_id,good_key)
);
CREATE TABLE processed_sitos_ticks (
 event_id BIGINT NOT NULL, settlement_id UUID NOT NULL,
 processed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 PRIMARY KEY(event_id,settlement_id)
);
