-- Lawagetas' first steps card (megaron_radgivaren.md): per player per world, the
-- player may hide it. Computed-on-read otherwise; this is the only stored bit.
ALTER TABLE player_world_records ADD COLUMN first_steps_hidden boolean NOT NULL DEFAULT false;
