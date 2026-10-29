-- Market board (megaron_plan_marknadstavlan, Timothy 2026-10-28): the ruler
-- writes what the city buys and sells; other rulers read it on site and keep
-- the reading as memory. The system computes nothing from stock or rates.
CREATE TABLE market_boards (
    settlement_id UUID PRIMARY KEY REFERENCES settlements(id) ON DELETE CASCADE,
    world_id      UUID NOT NULL REFERENCES worlds(id) ON DELETE CASCADE,
    buys          TEXT[] NOT NULL DEFAULT '{}',
    sells         TEXT[] NOT NULL DEFAULT '{}',
    written_tick  INT NOT NULL
);

-- One reading per reader and city; a newer reading replaces an older one.
-- written_tick is NULL when the board was read empty (never written).
CREATE TABLE market_board_readings (
    world_id      UUID NOT NULL REFERENCES worlds(id) ON DELETE CASCADE,
    player_id     UUID NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    settlement_id UUID NOT NULL REFERENCES settlements(id) ON DELETE CASCADE,
    buys          TEXT[] NOT NULL DEFAULT '{}',
    sells         TEXT[] NOT NULL DEFAULT '{}',
    written_tick  INT,
    read_tick     INT NOT NULL,
    PRIMARY KEY (player_id, settlement_id)
);

-- Hexes a player knows of only by word (a market board read from afar; later
-- a told story). Drawn as memory and open to letters, never to trade offers:
-- VisibleOrigins does not read this table (Timothy 2026-10-28 20:50/20:53).
CREATE TABLE player_heard_tiles (
    world_id   UUID NOT NULL REFERENCES worlds(id) ON DELETE CASCADE,
    player_id  UUID NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    q          INT NOT NULL,
    r          INT NOT NULL,
    source     TEXT NOT NULL CHECK (source IN ('board', 'told')),
    heard_tick INT NOT NULL,
    PRIMARY KEY (world_id, player_id, q, r)
);

-- Written on every delivery since mig 016, read by nothing; it copied computed
-- stock, which the 2026-10-22 decision (no Wants list) retired.
DROP TABLE market_snapshots;
