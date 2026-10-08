-- Slice T (megaron_transportrisk.md, Timothy 2026-10-08): every ship at sea
-- risks a storm for each sea hex it enters. SeaStormScanHandler rolls each
-- entered hex exactly once; this row is the per-voyage high-water mark
-- (index into the saved route), written in the same TX as the hull outcome.
-- voyage_key: 'u:<unit>:<depart_tick>:<arrive_tick>' for a marching ship,
-- 't:<transport>' for a ship bound to a transport leg.
CREATE TABLE sea_storm_progress (
    voyage_key   TEXT        PRIMARY KEY,
    world_id     UUID        NOT NULL REFERENCES worlds(id) ON DELETE CASCADE,
    ship_unit_id UUID        NOT NULL REFERENCES units(id) ON DELETE CASCADE,
    last_hex     INT         NOT NULL,
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_sea_storm_progress_world ON sea_storm_progress (world_id);
