-- Del 2b: sack/plunder vs annex — the capture choice rides on the marching unit.
--
-- Timothy 2026-07-10: conquest becomes a CHOICE — 'sack' (default) loots goods and
-- razes the settlement, 'annex' keeps today's behaviour (capital→colony takeover).
-- Set at march dispatch (POST /march "mode"), read by combat.resolveCombat's two
-- victory paths (unit_arrival.go).
ALTER TABLE units ADD COLUMN capture_mode TEXT NOT NULL DEFAULT 'sack';
