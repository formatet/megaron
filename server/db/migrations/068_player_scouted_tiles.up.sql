CREATE TABLE player_scouted_tiles (
    world_id   UUID        NOT NULL,
    player_id  UUID        NOT NULL REFERENCES players(id),
    q          INT         NOT NULL,
    r          INT         NOT NULL,
    scouted_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (world_id, player_id, q, r)
);
CREATE INDEX ON player_scouted_tiles (world_id, player_id);
