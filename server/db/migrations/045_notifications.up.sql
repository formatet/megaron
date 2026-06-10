CREATE TABLE notifications (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    world_id    UUID NOT NULL REFERENCES worlds(id) ON DELETE CASCADE,
    player_id   UUID NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    kind        TEXT NOT NULL,
    level       INT  NOT NULL,
    body_json   JSONB NOT NULL DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    read_at     TIMESTAMPTZ
);

CREATE INDEX ON notifications(player_id, world_id) WHERE read_at IS NULL;
