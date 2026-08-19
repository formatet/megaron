ALTER TABLE market_snapshots ADD COLUMN price FLOAT NOT NULL DEFAULT 0;
ALTER TABLE market_snapshots DROP COLUMN rate;
