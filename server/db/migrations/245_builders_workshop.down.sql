DROP TRIGGER build_building_change ON buildings;
DROP TRIGGER build_placement_change ON settlement_placement;
DROP FUNCTION build_city_changed();
DROP FUNCTION sync_build_queue(uuid);
DROP FUNCTION build_rate(uuid);
DROP FUNCTION build_work_left(integer,integer,integer,integer);
ALTER TABLE build_queue DROP COLUMN work_remaining, DROP COLUMN work_calc_tick, DROP COLUMN work_rate, DROP COLUMN work_revision;
DROP FUNCTION shipyard_work_left(integer,integer,integer,integer);
CREATE FUNCTION shipyard_work_left(work integer, running boolean, calc integer, at_tick integer)
RETURNS integer LANGUAGE sql IMMUTABLE AS $$ SELECT greatest(0,work-CASE WHEN running THEN greatest(0,at_tick-calc) ELSE 0 END) $$;
ALTER TABLE units DROP COLUMN shipyard_work_rate;
DELETE FROM goods WHERE key='building';
-- sync_shipyard_work keeps the rate body; restore mig 204's by re-running its CREATE if a true rollback is needed.
