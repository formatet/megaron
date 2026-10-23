-- Refuse a destructive rollback of live work; finish/cancel jobs first.
DO $$ BEGIN IF EXISTS(SELECT 1 FROM units WHERE shipyard_work_remaining IS NOT NULL) THEN RAISE EXCEPTION 'active shipyard work must be resolved before rollback'; END IF; END $$;
DROP TRIGGER shipyard_unit_change ON units;
DROP TRIGGER shipyard_population_owner_change ON settlements;
DROP TRIGGER shipyard_building_change ON buildings;
DROP TRIGGER shipyard_placement_change ON settlement_placement;
DROP FUNCTION shipyard_unit_changed();
DROP FUNCTION shipyard_city_changed();
DROP FUNCTION sync_shipyard_work(uuid);
DROP FUNCTION shipyard_work_left(integer,boolean,integer,integer);
DROP TABLE shipyard_work_transition;
DROP INDEX scheduled_shipyard_unit_pending;
ALTER TABLE units DROP COLUMN shipyard_work_remaining,DROP COLUMN shipyard_work_calc_tick,DROP COLUMN shipyard_work_running,DROP COLUMN shipyard_work_revision;
DELETE FROM settlement_placement WHERE good_key='shipbuilding';
DELETE FROM goods WHERE key='shipbuilding';
