-- Market snapshots: what a player knows about another settlement's goods.
-- Updated when a caravan delivers to or a messenger arrives at a settlement.
-- Own and allied settlements always show live data in the UI.
CREATE TABLE market_snapshots (
    player_id     UUID NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    settlement_id UUID NOT NULL REFERENCES settlements(id) ON DELETE CASCADE,
    good_key      TEXT NOT NULL,
    stock         FLOAT NOT NULL,
    price         FLOAT NOT NULL,
    observed_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (player_id, settlement_id, good_key)
);
CREATE INDEX idx_market_snapshots_player ON market_snapshots (player_id, observed_at DESC);
