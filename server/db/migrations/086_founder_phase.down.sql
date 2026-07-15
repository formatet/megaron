-- Revert 086: drop the founder phase.
--
-- Host tokens themselves are ordinary units rows (type='nomadic_host'); they are
-- left alone here, as 084 left renamed types alone. A world still mid-founding
-- when this is rolled back keeps orphan host tokens with no state behind them —
-- clean those manually if it ever happens (no forward path relies on it).

DROP INDEX IF EXISTS idx_founder_phase_host_unit;
DROP INDEX IF EXISTS idx_founder_phase_active;
DROP TABLE IF EXISTS founder_phase;
