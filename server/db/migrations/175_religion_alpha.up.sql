-- Alpha prayers have explicit world-tick cooldowns per god and Wanax.
-- Old RiteCast events and retired prayer effects are never rewritten.
CREATE TABLE prayer_cooldowns (
 world_id UUID NOT NULL REFERENCES worlds(id) ON DELETE CASCADE,
 player_id UUID NOT NULL REFERENCES players(id) ON DELETE CASCADE,
 god_id TEXT NOT NULL,
 until_tick INT NOT NULL,
 PRIMARY KEY(world_id, player_id, god_id)
);
ALTER TABLE settlements ADD COLUMN garrison_defense_until_tick INT;
ALTER TABLE settlements ADD COLUMN garrison_defense_owner_id UUID REFERENCES players(id) ON DELETE SET NULL;
