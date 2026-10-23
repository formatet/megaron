-- Shipbuilding is service labour, never inventory. This parked FK anchor follows
-- cult's placement-role precedent; it has no production rule or tradeability.
INSERT INTO goods(key,name,tier,category,base_value,weight,status)
VALUES('shipbuilding','Shipyard work','manufactured','bulk',1,0,'parked');
ALTER TABLE units ADD COLUMN shipyard_work_remaining integer CHECK(shipyard_work_remaining>=0),
 ADD COLUMN shipyard_work_calc_tick integer,
 ADD COLUMN shipyard_work_running boolean NOT NULL DEFAULT false,
 ADD COLUMN shipyard_work_revision integer NOT NULL DEFAULT 0;
CREATE INDEX units_shipyard_work_city ON units(settlement_id) WHERE shipyard_work_remaining IS NOT NULL;
CREATE INDEX scheduled_shipyard_unit_pending ON scheduled_events((payload->>'unit_id')) WHERE event_type='ShipyardWorkComplete' AND processed_at IS NULL;
CREATE TABLE shipyard_work_transition(id boolean PRIMARY KEY DEFAULT true CHECK(id),completed boolean NOT NULL DEFAULT false);
INSERT INTO shipyard_work_transition VALUES(true,false);

CREATE FUNCTION shipyard_work_left(work integer, running boolean, calc integer, at_tick integer)
RETURNS integer LANGUAGE sql IMMUTABLE AS $$ SELECT greatest(0,work-CASE WHEN running THEN greatest(0,at_tick-calc) ELSE 0 END) $$;

-- Settle the OLD saved rate before deriving the new rate. All writers (including
-- raw SQL population/ownership changes) enter this owner in the same transaction.
CREATE FUNCTION sync_shipyard_work(ship uuid) RETURNS void LANGUAGE plpgsql AS $$
DECLARE u units%ROWTYPE; t integer; work integer; running boolean; rev integer;
BEGIN
 SELECT * INTO u FROM units WHERE id=ship FOR UPDATE;
 IF NOT FOUND OR u.shipyard_work_remaining IS NULL THEN RETURN; END IF;
 SELECT current_tick INTO t FROM worlds WHERE id=u.world_id;
 work:=shipyard_work_left(u.shipyard_work_remaining,u.shipyard_work_running,u.shipyard_work_calc_tick,t);
 running:=u.category='naval' AND u.status IN ('forming','repairing') AND EXISTS(
  SELECT 1 FROM settlements s JOIN buildings b ON b.settlement_id=s.id AND b.building_type='shipyard' AND b.level>=1
  JOIN settlement_placement p ON p.settlement_id=s.id AND p.target_kind='building' AND p.building_type='shipyard' AND p.good_key='shipbuilding'
  WHERE s.id=u.settlement_id AND s.owner_id=u.owner_id AND s.state='active' AND s.population>=100);
 IF u.status NOT IN ('forming','repairing') OR u.category<>'naval' THEN
  UPDATE units SET shipyard_work_remaining=NULL,shipyard_work_running=false,build_complete_at=NULL WHERE id=ship;
  UPDATE scheduled_events SET processed_at=now() WHERE event_type='ShipyardWorkComplete' AND payload->>'unit_id'=ship::text AND processed_at IS NULL;
  RETURN;
 END IF;
 IF t=u.shipyard_work_calc_tick AND running=u.shipyard_work_running THEN RETURN; END IF;
 rev:=u.shipyard_work_revision+1;
 UPDATE units SET shipyard_work_remaining=work,shipyard_work_calc_tick=t,shipyard_work_running=running,shipyard_work_revision=rev,build_complete_at=NULL WHERE id=ship;
 UPDATE scheduled_events SET processed_at=now() WHERE event_type='ShipyardWorkComplete' AND payload->>'unit_id'=ship::text AND processed_at IS NULL;
 IF running THEN
  INSERT INTO scheduled_events(world_id,event_type,payload,process_after,due_tick,priority)
  VALUES(u.world_id,'ShipyardWorkComplete',jsonb_build_object('unit_id',ship,'revision',rev),now(),t+work,10);
 END IF;
END $$;

CREATE FUNCTION shipyard_city_changed() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE city uuid; ship uuid;
BEGIN
 IF TG_TABLE_NAME='buildings' THEN
  IF TG_OP='DELETE' THEN
   IF OLD.building_type='shipyard' THEN
    DELETE FROM settlement_placement WHERE settlement_id=OLD.settlement_id AND building_type='shipyard';
   END IF;
  END IF;
 END IF;
 IF TG_TABLE_NAME='settlements' THEN city:=NEW.id;
 ELSIF TG_OP='DELETE' THEN city:=OLD.settlement_id;
 ELSE city:=NEW.settlement_id; END IF;
 FOR ship IN SELECT id FROM units WHERE settlement_id=city AND shipyard_work_remaining IS NOT NULL ORDER BY id LOOP
  PERFORM sync_shipyard_work(ship);
 END LOOP;
 IF TG_TABLE_NAME <> 'settlements' THEN
 IF TG_OP='UPDATE' THEN
 IF OLD.settlement_id IS DISTINCT FROM NEW.settlement_id THEN
  FOR ship IN SELECT id FROM units WHERE settlement_id=OLD.settlement_id AND shipyard_work_remaining IS NOT NULL ORDER BY id LOOP
   PERFORM sync_shipyard_work(ship);
  END LOOP;
 END IF;
 END IF;
 END IF;
 RETURN NULL;
END $$;
CREATE TRIGGER shipyard_placement_change AFTER INSERT OR UPDATE OR DELETE ON settlement_placement FOR EACH ROW EXECUTE FUNCTION shipyard_city_changed();
CREATE TRIGGER shipyard_building_change AFTER INSERT OR UPDATE OR DELETE ON buildings FOR EACH ROW EXECUTE FUNCTION shipyard_city_changed();
CREATE TRIGGER shipyard_population_owner_change AFTER UPDATE OF population,owner_id,state ON settlements FOR EACH ROW EXECUTE FUNCTION shipyard_city_changed();
CREATE FUNCTION shipyard_unit_changed() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN PERFORM sync_shipyard_work(NEW.id); RETURN NULL; END $$;
CREATE TRIGGER shipyard_unit_change AFTER UPDATE OF owner_id,status,settlement_id ON units FOR EACH ROW EXECUTE FUNCTION shipyard_unit_changed();
