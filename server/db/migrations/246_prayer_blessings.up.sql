-- Prayers with a target (bönesystemet v3 steg 2a, Timothy 2026-10-31/11-06).
-- A blessing is read when it is needed (battle, storm), wherever the unit is.
--   scope 'unit'  : one named unit of the owner      (unit_id)
--   scope 'city'  : units supported by one city       (settlement_id = units.support_settlement_id, permanent)
--   scope 'realm' : every unit of the owner
-- from_tick = the cast tick, so a storm met BEFORE the prayer is never covered.
-- A ward (the hull prayer) is spent once per ship: prayer_blessing_wards_used carries
-- all consumption, for every scope.
CREATE TABLE prayer_blessings (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    world_id      uuid NOT NULL REFERENCES worlds(id) ON DELETE CASCADE,
    owner_id      uuid NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    effect        text NOT NULL,
    scope         text NOT NULL CHECK (scope IN ('unit','city','realm')),
    unit_id       uuid REFERENCES units(id) ON DELETE CASCADE,
    settlement_id uuid REFERENCES settlements(id) ON DELETE CASCADE,
    from_tick     integer NOT NULL,
    until_tick    integer NOT NULL,
    cast_event_id bigint,
    CHECK ((scope = 'unit') = (unit_id IS NOT NULL)),
    CHECK ((scope = 'city') = (settlement_id IS NOT NULL))
);
CREATE INDEX prayer_blessings_owner_effect ON prayer_blessings(owner_id, effect, until_tick);

CREATE TABLE prayer_blessing_wards_used (
    blessing_id uuid NOT NULL REFERENCES prayer_blessings(id) ON DELETE CASCADE,
    unit_id     uuid NOT NULL REFERENCES units(id) ON DELETE CASCADE,
    PRIMARY KEY (blessing_id, unit_id)
);
