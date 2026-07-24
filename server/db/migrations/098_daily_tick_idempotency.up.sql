-- Migration 098: idempotency claims for the kharis + colony daily ticks
--
-- Sibling of 097 (processed_sitos_ticks). The same Fas 2.2 gap that sitos had
-- was left in two more daily handlers, found while fixing sitos and deferred
-- then as out of scope:
--
--   internal/kharis/tick.go     TickHandler.Handle        -> processMaintenance
--   internal/loyalty/colony.go  ColonyPenaltyHandler.Handle -> applyColonyPenalty
--
-- Both fan ONE scheduled event out across many rows and mutate directly, with
-- no claim and no FOR UPDATE. events.Worker marks a scheduled event done in a
-- SEPARATE statement AFTER Handle returns (events/scheduler.go markDone), so
-- any crash, transient error, or G2 handler timeout between the first row's
-- write and Handle finishing leaves the event unprocessed — it is re-claimed
-- and re-run from the top, re-applying to every row already done. Kharis
-- re-runs the day's decay/gain and re-charges the temple offering; colony
-- appends a SECOND colony_penalty loyalty_event, and loyalty is the one
-- projection that IS replay-derived (settlement/loyalty.go), so the duplicate
-- does not just log twice, it moves the value twice.
--
-- The 5s G2 timeout makes this reachable, not theoretical: both handlers scan
-- per player and per settlement, so the wider the world, the likelier a pass
-- exceeds the deadline mid-fan-out. A fleet grown to 11 wanaxes runs straight
-- at it.
--
-- One table for both, keyed (event_id, scope_id), rather than a table per
-- handler: the claim shape is identical and the retention question these
-- tables raise (they grow one row per event/scope forever, todo §Teknisk skuld)
-- is easier to answer once than three times. scope_id is whatever the handler
-- commits per: player_id for kharis, settlement_id for colony. They cannot
-- collide — the key includes event_id, and one event belongs to one handler.

CREATE TABLE processed_tick_claims (
    event_id     BIGINT NOT NULL,
    scope_id     UUID   NOT NULL,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (event_id, scope_id)
);

COMMENT ON TABLE processed_tick_claims IS
    'Exactly-once claims for daily tick handlers that fan one event across many rows (kharis, colony). scope_id = player_id or settlement_id depending on handler. See migration 098.';
