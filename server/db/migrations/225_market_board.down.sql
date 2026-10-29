DROP TABLE player_heard_tiles;
DROP TABLE market_board_readings;
DROP TABLE market_boards;
-- market_snapshots is not recreated with its rows: it held no information any
-- code read. The empty table is restored so older binaries can still write it.
CREATE TABLE market_snapshots (
    player_id     UUID NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    settlement_id UUID NOT NULL REFERENCES settlements(id) ON DELETE CASCADE,
    good_key      TEXT NOT NULL,
    stock         FLOAT NOT NULL,
    observed_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    secondhand    BOOLEAN NOT NULL DEFAULT false,
    rate          FLOAT NOT NULL DEFAULT 0,
    PRIMARY KEY (player_id, settlement_id, good_key)
);
