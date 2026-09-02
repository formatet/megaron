-- Reverse of 138: recreate the outpost substrate with the schema it had
-- immediately before the drop (unchanged since 030_outpost). Data is not
-- recoverable — these tables/columns were never populated in the first place.
ALTER TABLE provinces
    ADD COLUMN owner_id UUID REFERENCES players(id),
    ADD COLUMN outpost_feeds UUID REFERENCES settlements(id),
    ADD COLUMN garrison_strength INT NOT NULL DEFAULT 0;

CREATE TABLE outpost_flows (
    province_id  UUID    NOT NULL REFERENCES provinces(id) ON DELETE CASCADE,
    good_key     TEXT    NOT NULL,
    world_id     UUID    NOT NULL REFERENCES worlds(id) ON DELETE CASCADE,
    settlement_id UUID   NOT NULL REFERENCES settlements(id) ON DELETE CASCADE,
    rate         DOUBLE PRECISION NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (province_id, good_key)
);

CREATE INDEX idx_outpost_flows_settlement ON outpost_flows (settlement_id);

CREATE INDEX idx_provinces_outpost ON provinces (world_id)
    WHERE controller_id IS NULL AND owner_id IS NOT NULL;
