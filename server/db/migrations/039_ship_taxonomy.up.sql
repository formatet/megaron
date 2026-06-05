-- Migration 039: Skepp-taxonomi — galley/war_galley/merchantman
-- Kolumnen `ship` behålls intakt (87 Go-referenser; rename för bräckligt).
-- Display-namn "Galley" mappas i UI/kod; ship-kolumnen är kanonisk.
-- Nya kolumner: war_galley + merchantman på settlements + marching_armies.

ALTER TABLE settlements
    ADD COLUMN war_galley   INT NOT NULL DEFAULT 0,
    ADD COLUMN merchantman  INT NOT NULL DEFAULT 0;

ALTER TABLE marching_armies
    ADD COLUMN war_galley   INT NOT NULL DEFAULT 0,
    ADD COLUMN merchantman  INT NOT NULL DEFAULT 0;
