-- Foreign city contact (megaron_kontrakt_frammande_stad, Timothy 2026-10-30):
-- a snapshot taken when something of the player's own reaches another ruler's
-- city (messenger delivered, caravan/gift/trade delivered, market board read).
-- It never ages on its own and a newer contact replaces it. Fields are stored
-- ALREADY COARSE — words, never raw numbers — so they cannot leak later.
-- Written only by economy.RecordContact. No backfill: a contact before this
-- migration has no snapshot from its own day, and today's would carry a lie.
CREATE TABLE player_contacted_settlements (
    world_id      UUID NOT NULL REFERENCES worlds(id) ON DELETE CASCADE,
    player_id     UUID NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    settlement_id UUID NOT NULL REFERENCES settlements(id) ON DELETE CASCADE,
    contact_tick  INT NOT NULL,
    source        TEXT NOT NULL CHECK (source IN ('messenger', 'caravan', 'trade', 'market_board')),
    name          TEXT NOT NULL,
    owner_id      UUID,
    owner_name    TEXT NOT NULL DEFAULT '',
    size          TEXT NOT NULL,
    food          TEXT NOT NULL,
    goods         JSONB NOT NULL DEFAULT '[]',
    buildings     JSONB NOT NULL DEFAULT '[]',
    PRIMARY KEY (player_id, settlement_id)
);
