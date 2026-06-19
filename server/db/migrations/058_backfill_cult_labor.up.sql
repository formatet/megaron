-- Migration 058: backfill baseline cult labor weight for existing settlements.
-- Root cause: agents called LaborAlloc with grain-only allocations, which DELETE'd
-- all settlement_labor rows and re-inserted only the agent-chosen goods — leaving
-- cult weight at zero and temples permanently inert (cult_rate = 0 → kharis stagnates).
--
-- Fix: insert cult weight 0.15 for every settlement that has a temple building
-- but no cult labor row. Idempotent via ON CONFLICT DO NOTHING.
-- The 0.15 weight is additive (does not reduce grain workers), so grain
-- self-sufficiency is unaffected.
INSERT INTO settlement_labor (settlement_id, good_key, weight)
SELECT b.settlement_id, 'cult', 0.15
FROM buildings b
WHERE b.building_type = 'temple'
  AND NOT EXISTS (
      SELECT 1 FROM settlement_labor sl
      WHERE sl.settlement_id = b.settlement_id
        AND sl.good_key = 'cult'
  )
ON CONFLICT (settlement_id, good_key) DO NOTHING;
