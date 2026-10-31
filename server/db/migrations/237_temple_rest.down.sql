CREATE TABLE prayer_cooldowns (
 world_id UUID NOT NULL REFERENCES worlds(id) ON DELETE CASCADE,
 player_id UUID NOT NULL REFERENCES players(id) ON DELETE CASCADE,
 god_id TEXT NOT NULL,
 until_tick INT NOT NULL,
 PRIMARY KEY(world_id, player_id, god_id)
);
DROP TABLE temple_rest;
