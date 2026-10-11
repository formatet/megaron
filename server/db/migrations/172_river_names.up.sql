-- Static river chains keep their canonical anchor. No rights are backfilled.
CREATE TABLE river_names (
 world_id UUID NOT NULL REFERENCES worlds(id) ON DELETE CASCADE,
 anchor_q INT NOT NULL,
 anchor_r INT NOT NULL,
 claimed_by UUID REFERENCES players(id) ON DELETE SET NULL,
 claimed_tick INT,
 name TEXT,
 named_tick INT,
 PRIMARY KEY(world_id,anchor_q,anchor_r)
);
CREATE UNIQUE INDEX idx_river_names_name ON river_names(world_id,lower(name)) WHERE name IS NOT NULL;
