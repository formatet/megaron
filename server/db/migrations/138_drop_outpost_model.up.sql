-- Migration 138: drop the dead outpost data model (Timothy 2026-09-02: "radera").
--
-- Outposts have long been a killed mechanic in the design docs — the player
-- has no way to create one, and it has no live role. This migration finally
-- removes the substrate migration 030 built for it: `outpost_flows` and the
-- three `provinces` columns it added (`owner_id`, `outpost_feeds`,
-- `garrison_strength`), plus their indexes.
--
-- Verified dead against master (this branch), not just by name:
--   - provinces.outpost_feeds is NEVER set to a non-NULL value anywhere in Go
--     code — every write is `outpost_feeds = NULL` (settlement.go Abandon,
--     combat/collapse.go, messenger/recall.go). The only read was
--     api/handlers/world.go's "outpost provinces" map-marker query, which by
--     construction (`WHERE outpost_feeds IS NOT NULL`) always matched zero
--     rows and is removed alongside this migration.
--   - provinces.owner_id (added by 030, NOT the same column as
--     settlements.owner_id from migration 005) is likewise never set to a
--     non-NULL value anywhere — every write is `owner_id = NULL`, and the only
--     read was the same dead world.go query.
--   - provinces.garrison_strength is never read anywhere (no SELECT) and only
--     ever written as `garrison_strength = 0`.
--   - outpost_flows has no INSERT anywhere in Go code — every reference
--     (combat/collapse.go, messenger/recall.go, settlement.go Abandon) only
--     SELECTs (always zero rows) or DELETEs an already-empty table.
--   - messenger/recall.go's "outpost" recall sub-type (RecallOutpostPayload,
--     handleOutpost) had zero producers: the only site that schedules a
--     RecallArrival event (api/handlers/province.go RecallMarch) always sets
--     Kind: "march". Removed as dead code alongside this migration.
--
-- `settlement.go`'s Abandon handler, `combat/collapse.go`'s teardown, and
-- `messenger/recall.go`'s handleMarch/RecallMarchPayload path are UNTOUCHED —
-- they handle real settlement/army lifecycle, not outposts.
DROP INDEX IF EXISTS idx_provinces_outpost;
DROP INDEX IF EXISTS idx_outpost_flows_settlement;
DROP TABLE IF EXISTS outpost_flows;
ALTER TABLE provinces
    DROP COLUMN IF EXISTS garrison_strength,
    DROP COLUMN IF EXISTS outpost_feeds,
    DROP COLUMN IF EXISTS owner_id;
