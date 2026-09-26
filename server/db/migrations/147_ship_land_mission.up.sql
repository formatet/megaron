-- Migration 147: skeppsuppdrag — landsätt (megaron_plan_skeppsuppdrag_landsatt.md R1).
--
-- A ship given a "land" mission (POST .../march intent=land) sails to the sea hex next
-- to a chosen LAND target, then lands its cargo there and turns for home on its own
-- (dispatchReturnHome reuses the explore_return machinery). The march itself redirects
-- units.target_q/target_r to that sea waypoint (same trick assault already uses), so the
-- true land target has to be remembered somewhere else — these two columns hold it.
-- land_cargo_intent carries the optional "colonize" choice made at dispatch; the chosen
-- colony name reuses the existing colony_name column (mig 059) — no new column for that.
ALTER TABLE units ADD COLUMN land_target_q INT;
ALTER TABLE units ADD COLUMN land_target_r INT;
ALTER TABLE units ADD COLUMN land_cargo_intent TEXT;
