-- Bönegrunden (bönesystem v3, slice 1): a temple rests after it answers, not a
-- god per Wanax. The rest grows with the size of the gift and shrinks with the
-- temple's level; it only counts down while the temple is fully staffed
-- (kharis tick, last_tick guards double counting). Rows are deleted at 0.
-- prayer_cooldowns (god x Wanax) is retired; alpha accepts that any waiting
-- time in progress is released.
CREATE TABLE temple_rest (
 building_id UUID PRIMARY KEY REFERENCES buildings(id) ON DELETE CASCADE,
 remaining_ticks INT NOT NULL,
 last_tick INT NOT NULL
);
DROP TABLE prayer_cooldowns;
