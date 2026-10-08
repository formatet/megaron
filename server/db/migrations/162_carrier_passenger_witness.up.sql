-- T2: messenger owns its projection of the physical carrier's durable witness.
-- No FK: archive retention must not prevent deleting old domain events.
ALTER TABLE messengers ADD COLUMN carrier_witness_id BIGINT NOT NULL DEFAULT 0;
CREATE INDEX events_carrier_passenger_stream ON events (stream_id, id)
WHERE event_type IN ('CarrierPassengerLostV1', 'CarrierPassengerRescuedV1', 'CarrierPassengerLandedV1', 'CarrierPassengerRedirectedV1');
