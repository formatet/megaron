-- Rollback migration 039: ta bort war_galley + merchantman

ALTER TABLE settlements
    DROP COLUMN IF EXISTS war_galley,
    DROP COLUMN IF EXISTS merchantman;

ALTER TABLE marching_armies
    DROP COLUMN IF EXISTS war_galley,
    DROP COLUMN IF EXISTS merchantman;
