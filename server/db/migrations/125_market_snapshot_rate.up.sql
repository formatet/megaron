ALTER TABLE market_snapshots ADD COLUMN rate FLOAT NOT NULL DEFAULT 0;
ALTER TABLE market_snapshots DROP COLUMN price;
