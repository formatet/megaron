-- Gossip propagation through the contact graph (rumor chains) + secondhand
-- market knowledge. See temenos_gossip.md.

ALTER TABLE gossip_events ADD COLUMN rumor_id UUID;
ALTER TABLE gossip_events ADD COLUMN hops INT NOT NULL DEFAULT 0;

-- Dedup: a rumor (identified by its shared rumor_id) reaches a given recipient
-- at most once, however many contact paths it travels through.
CREATE UNIQUE INDEX idx_gossip_events_recipient_rumor
    ON gossip_events (recipient_id, rumor_id) WHERE rumor_id IS NOT NULL;

ALTER TABLE market_snapshots ADD COLUMN secondhand BOOLEAN NOT NULL DEFAULT false;
