-- Lawagetas' first steps card (megaron_radgivaren.md): per player per world.
-- first_steps_hidden: the player hid the card. first_steps_completed: the four
-- steps were all done once — the advice then ends for good (a later step that
-- flips back, e.g. a free citizen after growth, must not bring the card back).
-- Everything else is computed on read.
ALTER TABLE player_world_records
    ADD COLUMN first_steps_hidden boolean NOT NULL DEFAULT false,
    ADD COLUMN first_steps_completed boolean NOT NULL DEFAULT false;
