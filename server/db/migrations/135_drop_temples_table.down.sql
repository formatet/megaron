-- Reverse of 135: recreate `temples` with the schema it had immediately
-- before the drop (post-005_province_settlement_split, post-110_drop_priest_columns:
-- province_id was migrated into settlement_id in 005, priest_id was dropped
-- in 110). Data is not recoverable.
CREATE TABLE temples (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    pantheon_id   TEXT NOT NULL,
    level         INT NOT NULL DEFAULT 1,
    local_power   FLOAT NOT NULL DEFAULT 0.5,
    built_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    settlement_id UUID REFERENCES settlements(id)
);
