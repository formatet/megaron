-- Stormbönerna (bönesystemet v3 steg 3b, Timothy 2026-11-07).
-- sea_storm_calms: one row per "Still the Storm" cast. The removed hex is the stored outcome of the
-- one roll; the weather step grows a hex back at regrow_tick (cast_tick + 24) and stamps regrown_tick.
-- cast_key is the idempotency key (carried in the RiteCast payload): the same key twice is one effect.
CREATE TABLE sea_storm_calms (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    storm_id     uuid NOT NULL REFERENCES sea_storms(id) ON DELETE CASCADE,
    cast_key     uuid NOT NULL UNIQUE,
    cast_tick    integer NOT NULL,
    regrow_tick  integer NOT NULL,
    regrown_tick integer,
    removed_q    integer NOT NULL,
    removed_r    integer NOT NULL
);
CREATE INDEX sea_storm_calms_pending ON sea_storm_calms(storm_id) WHERE regrown_tick IS NULL;

-- sea_storm_wakes: one row per "Wake the Storm" cast (audit + idempotency). Kept apart from
-- sea_storms on purpose: nothing that reads a storm can tell a woken one from a born one.
CREATE TABLE sea_storm_wakes (
    cast_key  uuid PRIMARY KEY,
    storm_id  uuid NOT NULL REFERENCES sea_storms(id) ON DELETE CASCADE,
    cast_tick integer NOT NULL,
    hex_q     integer NOT NULL,
    hex_r     integer NOT NULL
);
