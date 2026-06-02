DROP INDEX IF EXISTS idx_provinces_outpost;
DROP INDEX IF EXISTS idx_outpost_flows_settlement;
DROP TABLE IF EXISTS outpost_flows;
ALTER TABLE provinces
    DROP COLUMN IF EXISTS garrison_strength,
    DROP COLUMN IF EXISTS outpost_feeds,
    DROP COLUMN IF EXISTS owner_id;
