-- Migration 034: delivery idempotency marker
--
-- The events.Worker marks a scheduled event done in a SEPARATE statement AFTER
-- the handler's transaction commits. A crash between commit and markDone leaves
-- the event unprocessed → it gets re-claimed and re-run.
--
-- Route-based trade deliveries are guarded by trade_routes.resolved, but
-- messenger-trade legs (trade_route_id = zero UUID) had NO in-transaction guard:
-- a retry would double-credit silver to the seller AND double-schedule the goods
-- return. This violates the Fas 2.2 idempotency rule.
--
-- This table is the exactly-once claim. DeliveryHandler INSERTs the scheduled
-- event id inside its own transaction; the marker commits atomically with the
-- credit, so a retry sees the row and short-circuits.

CREATE TABLE processed_deliveries (
    event_id     BIGINT PRIMARY KEY,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
