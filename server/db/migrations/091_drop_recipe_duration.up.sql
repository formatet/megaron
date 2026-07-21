-- Drop the dead recipes.duration_min column (K1-svepet). Craft is instant — the
-- column was "reserved for future craft queues" (mig 010) but no code path ever
-- reads or writes it: the two runtime recipe reads (province.go, capabilities/
-- province_verbs.go) SELECT explicit columns (output_key, output_qty,
-- building_type) and recipes are seeded only by migrations. Removing it, not
-- keeping a phantom queue-timer field that lies about the mechanic.
ALTER TABLE recipes DROP COLUMN IF EXISTS duration_min;
