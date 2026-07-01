ALTER TABLE market_snapshots DROP COLUMN secondhand;

DROP INDEX idx_gossip_events_recipient_rumor;
ALTER TABLE gossip_events DROP COLUMN hops;
ALTER TABLE gossip_events DROP COLUMN rumor_id;
