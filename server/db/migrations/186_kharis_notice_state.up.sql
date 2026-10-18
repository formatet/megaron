-- Owner-scoped transition memory. Realm scope_id is the Wanax id; garrison
-- scope_id is the city id. Historical ownership never suppresses a new owner's notice.
CREATE TABLE kharis_notice_state (
 world_id UUID NOT NULL REFERENCES worlds(id) ON DELETE CASCADE,
 player_id UUID NOT NULL REFERENCES players(id) ON DELETE CASCADE,
 scope_id UUID NOT NULL,
 condition TEXT NOT NULL,
 state TEXT NOT NULL,
 observed_tick INT NOT NULL,
 -- Last owner-private context survives clearing/retaining the notification archive.
 context JSONB NOT NULL DEFAULT '{}'::jsonb,
 PRIMARY KEY(world_id,player_id,scope_id,condition)
);
