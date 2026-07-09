-- Reverse migration 075: restore the old kharis_cap default (2000).
--
-- NOTE: kharis_amount data is NOT restored to the old 0-2000 scale. The ÷20
-- compression applied by the up-migration is lossy with respect to "what the old
-- absolute number meant" (it was itself an artifact of the pegged-at-cap bug this
-- redesign fixes), and there is no canonical inverse once players have played on
-- the new 0-100 scale. If a genuine rollback of live data is needed, restore from
-- a pre-migration database snapshot instead of relying on this down-migration.

ALTER TABLE player_world_records ALTER COLUMN kharis_cap SET DEFAULT 2000;
