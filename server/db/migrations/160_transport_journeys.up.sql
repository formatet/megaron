-- Existing transports retain their committed ETA and explicit legacy reader.
ALTER TABLE transports ADD COLUMN journey JSONB, ADD COLUMN departed_tick INTEGER;
ALTER TABLE transports ADD CONSTRAINT transports_journey_departure CHECK ((journey IS NULL) = (departed_tick IS NULL));
