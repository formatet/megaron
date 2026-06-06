-- Sprint A2: cavalry → chariot, DROP catapult
-- Katapulten finns inte i bronsåldern; kavalleriet döps om till stridsvagn (chariot).

ALTER TABLE settlements
    RENAME COLUMN cavalry TO chariot;

ALTER TABLE settlements
    DROP COLUMN catapult;

ALTER TABLE marching_armies
    RENAME COLUMN cavalry TO chariot;

ALTER TABLE marching_armies
    DROP COLUMN catapult;

ALTER TABLE borrowed_armies
    RENAME COLUMN cavalry TO chariot;

ALTER TABLE borrowed_armies
    DROP COLUMN catapult;
