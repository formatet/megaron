-- Potnia's offensive prayer (boner steg 5c): a wall falls one level three ticks after the cast.
-- One row per cast that has been carried out, so a re-run of the scheduled event is a no-op.
CREATE TABLE wall_falls (
    cast_event_id BIGINT PRIMARY KEY,
    settlement_id UUID NOT NULL,
    fell_tick     INT  NOT NULL,
    wall_before   INT  NOT NULL,
    wall_after    INT  NOT NULL
);
