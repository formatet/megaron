ALTER TABLE borrowed_armies
    ADD COLUMN catapult INT NOT NULL DEFAULT 0;

ALTER TABLE borrowed_armies
    RENAME COLUMN chariot TO cavalry;

ALTER TABLE marching_armies
    ADD COLUMN catapult INT NOT NULL DEFAULT 0;

ALTER TABLE marching_armies
    RENAME COLUMN chariot TO cavalry;

ALTER TABLE settlements
    ADD COLUMN catapult INT NOT NULL DEFAULT 0;

ALTER TABLE settlements
    RENAME COLUMN chariot TO cavalry;
