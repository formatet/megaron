-- Account erasure by admin (megaron_plan_kontoradering.md, Timothy 2026-10-10).
-- Erasure anonymises the login and keeps the rows: 32 foreign keys point at
-- players, and the world's history (letters, battles, epitaphs) belongs to the
-- other players too. erased_at marks the account; agora_deactivate_pending hands
-- the Agora deactivation to the server's reconciliation loop.
ALTER TABLE players
    ADD COLUMN erased_at TIMESTAMPTZ,
    ADD COLUMN agora_deactivate_pending BOOLEAN NOT NULL DEFAULT false;
