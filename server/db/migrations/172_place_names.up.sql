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

-- Progress is per frozen journey, not visibility and not dispatch claims.
CREATE TABLE place_reach_progress (
 journey_key TEXT PRIMARY KEY,
 world_id UUID NOT NULL REFERENCES worlds(id) ON DELETE CASCADE,
 last_tick INT NOT NULL,
 last_milli BIGINT NOT NULL
);
CREATE TABLE place_reach_epoch (
 world_id UUID PRIMARY KEY REFERENCES worlds(id) ON DELETE CASCADE,
 first_tick INT NOT NULL,
 applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- Never replay pre-deploy entries; existing journeys start at the next tick.
INSERT INTO place_reach_epoch(world_id,first_tick) SELECT id,current_tick+1 FROM worlds;
