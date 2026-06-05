-- Persistent FOW contacts for completed explore marches.
-- When a ship finishes an explore and returns home, the scouted province
-- stays visible — same mechanic as messenger contacts.
CREATE TABLE player_scouted_provinces (
    world_id    UUID        NOT NULL,
    player_id   UUID        NOT NULL REFERENCES players(id),
    province_id UUID        NOT NULL REFERENCES provinces(id),
    scouted_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (world_id, player_id, province_id)
);
CREATE INDEX ON player_scouted_provinces (world_id, player_id);
