ALTER TABLE transports DROP CONSTRAINT transports_journey_departure;
ALTER TABLE transports DROP COLUMN journey, DROP COLUMN departed_tick;
