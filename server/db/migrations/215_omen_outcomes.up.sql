-- Owner-private observed outcome; old atmospheric rows keep NULL metadata.
ALTER TABLE gossip_events ADD COLUMN fact_data JSONB;
-- Transport gossip remembers its actual owner across contact propagation.
ALTER TABLE gossip_events ADD COLUMN transport_owner_id UUID REFERENCES players(id) ON DELETE SET NULL;
