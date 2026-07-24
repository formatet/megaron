-- Migration 097: Sitos tick idempotency marker
--
-- SitosTickHandler.Handle (internal/economy/sitos_tick.go) fans one
-- ScheduledSitosTick event out across every active settlement in the world,
-- committing each settlement's tax/release/stabilize writes in its OWN
-- transaction (tickSettlement). The events.Worker marks a scheduled event
-- done in a SEPARATE statement AFTER Handle returns (internal/events/
-- scheduler.go markDone) — a crash or transient error between one
-- settlement's commit and Handle finishing leaves the event unprocessed, so
-- it gets re-claimed and re-run from scratch, double-taxing/double-releasing
-- every settlement already committed. This violates the Fas 2.2 idempotency
-- rule (CLAUDE.md "Event handlers") — the handler carried a bare
-- `// TODO: idempotent` marker instead of a guard.
--
-- Because each settlement commits independently, the claim must be scoped
-- per (event_id, settlement_id), not per event_id alone (that's what
-- processed_deliveries is for, and it doesn't fit here — a Handle-level
-- claim would either falsely skip settlements never reached before a crash,
-- or falsely mark them done before their writes commit). tickSettlement
-- INSERTs its own (event_id, settlement_id) row inside the SAME transaction
-- as its tax/release/stabilize writes, so the claim and the mutation commit
-- atomically: a retry of the same event resumes exactly where it left off.

CREATE TABLE processed_sitos_ticks (
    event_id     BIGINT NOT NULL,
    settlement_id UUID NOT NULL,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (event_id, settlement_id)
);
