-- PASS 2b: rumour has weight (minor vs major) and can name a subject settlement,
-- which registers it as "rumour-known" for the recipient. See temenos_gossip.md
-- PASS 2b. market_snapshots.secondhand (mig 069) is left in place, vestigial —
-- Mechanism 1 (secondhand snapshot copying) is removed in code, not schema.

ALTER TABLE gossip_events ADD COLUMN importance TEXT NOT NULL DEFAULT 'minor';
ALTER TABLE gossip_events ADD COLUMN subject_settlement_id UUID REFERENCES settlements(id);
ALTER TABLE gossip_events ADD COLUMN industry_hint TEXT;

-- Rumour-known settlements: a fourth, weaker knowledge tier (seen/live > remembered
-- > contacted > rumour-known). Populated when a rumour naming subject_settlement_id
-- reaches a player (gossip.Broadcast / gossip.PropagateOnContact). This table only
-- ever stores level='rumour' — seen/remembered/contacted are derived from geometry
-- and contacts at read time, never written here.
CREATE TABLE known_settlements (
    world_id      UUID        NOT NULL,
    player_id     UUID        NOT NULL REFERENCES players(id),
    settlement_id UUID        NOT NULL REFERENCES settlements(id),
    level         TEXT        NOT NULL DEFAULT 'rumour',
    industry_hint TEXT,
    learned_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (world_id, player_id, settlement_id)
);
CREATE INDEX ON known_settlements (world_id, player_id);
