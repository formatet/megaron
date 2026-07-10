-- Down: rates are already settled; no-op (rates won't be restored).
-- Revert the Go catchment queries to the 6-neighbour form; rates will recompute
-- without the centre hex next tick.
SELECT 1;
