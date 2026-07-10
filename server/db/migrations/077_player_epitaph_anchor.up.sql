-- Metropolis-succession / game-over epitaph anchor.
--
-- When a Wanax loses their LAST settlement (collapse or conquest), the succession
-- helper (internal/combat/succession.go) records the fallen capital here so the
-- epitaph crawl can reconstruct the reign from that settlement's event stream
-- AFTER owner_id / settlement_id have been cleared. Nullable and only set on true
-- game-over — never on metropolis succession (where a colony is promoted instead).
ALTER TABLE player_world_records
    ADD COLUMN last_settlement_id UUID;
