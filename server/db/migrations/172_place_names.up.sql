-- Static geographic objects keep their canonical anchor. No rights are backfilled.
CREATE TABLE place_names (
 world_id UUID NOT NULL REFERENCES worlds(id) ON DELETE CASCADE,
 kind TEXT NOT NULL CHECK (kind IN ('river','massif','desert','forest')),
 anchor_q INT NOT NULL,
 anchor_r INT NOT NULL,
 claimed_by UUID REFERENCES players(id) ON DELETE SET NULL,
 claimed_tick INT,
 name TEXT,
 named_tick INT,
 PRIMARY KEY(world_id,kind,anchor_q,anchor_r)
);
CREATE UNIQUE INDEX idx_place_names_name ON place_names(world_id,lower(name)) WHERE name IS NOT NULL;
