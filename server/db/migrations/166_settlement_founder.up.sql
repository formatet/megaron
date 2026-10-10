-- Who founded a city, and when (megaron_sista_staden.md; Timothy 2026-10-10:
-- the epitaph tells the WHOLE Wanax's history, not only the last city's).
-- Events carry no player column and a city's owner changes on capture, so
-- nothing remembered the founder. founded_tick is game time (ticks), so the
-- epitaph can order founding lines among events by world_tick.
ALTER TABLE settlements
    ADD COLUMN founder_id UUID REFERENCES players(id),
    ADD COLUMN founded_tick INT DEFAULT current_world_tick();

-- Backfill. The founder is the former owner named by the city's FIRST capture,
-- if it was ever captured; otherwise its current owner (a collapsed or
-- abandoned city has none, and stays NULL). The founding tick is the city's
-- earliest event, or 0.
UPDATE settlements s SET
    founder_id = COALESCE(
        (SELECT (e.payload->>'former_owner')::uuid FROM events e
          WHERE e.stream_id = s.id AND e.event_type = 'SettlementCaptured'
            AND e.payload->>'former_owner' IS NOT NULL
          ORDER BY e.id LIMIT 1),
        s.owner_id),
    founded_tick = COALESCE((SELECT min(e.world_tick) FROM events e WHERE e.stream_id = s.id), 0);

CREATE INDEX idx_settlements_founder ON settlements (world_id, founder_id);
