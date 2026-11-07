-- The bird oracle (boner steg 5d, Asterion): a god's live sight over one named place for a game day.
-- from_milli/until_milli are thousandths of a tick (tick.Anchor.MilliAt); the eye is active while
-- from_milli <= at < until_milli. One row per cast.
CREATE TABLE oracle_sights (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    world_id      UUID NOT NULL REFERENCES worlds(id) ON DELETE CASCADE,
    player_id     UUID NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    q             INT  NOT NULL,
    r             INT  NOT NULL,
    place_kind    TEXT NOT NULL,
    place_name    TEXT NOT NULL,
    from_milli    BIGINT NOT NULL,
    until_milli   BIGINT NOT NULL
);
CREATE INDEX oracle_sights_player ON oracle_sights (world_id, player_id, until_milli);
