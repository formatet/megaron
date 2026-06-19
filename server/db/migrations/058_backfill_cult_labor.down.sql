-- Reverse migration 058: remove the backfilled cult labor weights.
-- Only removes rows with exactly weight 0.15 (the backfill default) to avoid
-- deleting rows a Wanax/agent subsequently updated to a higher value.
DELETE FROM settlement_labor
WHERE good_key = 'cult'
  AND weight = 0.15;
