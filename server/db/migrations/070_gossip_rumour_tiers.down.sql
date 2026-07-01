DROP TABLE IF EXISTS known_settlements;
ALTER TABLE gossip_events DROP COLUMN industry_hint;
ALTER TABLE gossip_events DROP COLUMN subject_settlement_id;
ALTER TABLE gossip_events DROP COLUMN importance;
