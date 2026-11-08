-- The yard shares its pace (Timothy 2026-11-07): the placed shipyard workers (max 3) are divided
-- equally between every ship the yard is working on in the city (building or repairing), so three
-- workers and three ships is one third of a worker per ship.
--   Stored work is now in 60ths of a worker-tick (economy.ShipyardWorkParts); units.shipyard_work_rate
--   is floor(workers*60/ships), 0..180. Paid work is still given in worker-ticks (the Go side
--   multiplies), so one ship with one worker finishes on the same tick as before.
--   sync_shipyard_city(city) re-derives every ship of the city: a ship that starts, finishes, leaves or
--   changes hands changes everyone's share, so the unit trigger now syncs the whole city.
--   The sync functions never write owner_id/status/settlement_id, so that trigger cannot recurse.
ALTER TABLE units DROP CONSTRAINT units_shipyard_work_rate_check;
ALTER TABLE units ADD CONSTRAINT units_shipyard_work_rate_check CHECK(shipyard_work_rate BETWEEN 0 AND 180);

-- Placed shipbuilding workers of a city for a ship of this owner (0 when the yard cannot work).
CREATE FUNCTION shipyard_workers(city uuid, ship_owner uuid) RETURNS integer LANGUAGE sql STABLE AS $$
 SELECT least(3,count(*))::integer FROM settlements s
 JOIN buildings b ON b.settlement_id=s.id AND b.building_type='shipyard' AND b.level>=1
 JOIN settlement_placement p ON p.settlement_id=s.id AND p.target_kind='building' AND p.building_type='shipyard' AND p.good_key='shipbuilding'
 WHERE s.id=city AND s.owner_id=ship_owner AND s.state='active' AND s.population>=100 $$;

-- Ships the yard of this city is working on (builds and repairs), at least 1.
CREATE FUNCTION shipyard_sharing(city uuid, ship_owner uuid) RETURNS integer LANGUAGE sql STABLE AS $$
 SELECT greatest(1,count(*))::integer FROM units v
 WHERE v.settlement_id=city AND v.owner_id=ship_owner AND v.category='naval'
   AND v.status IN ('forming','repairing') AND v.shipyard_work_remaining IS NOT NULL $$;

CREATE OR REPLACE FUNCTION sync_shipyard_work(ship uuid) RETURNS void LANGUAGE plpgsql AS $$
DECLARE u units%ROWTYPE; t integer; work integer; rate integer; rev integer; scale boolean;
BEGIN
 SELECT * INTO u FROM units WHERE id=ship FOR UPDATE;
 IF NOT FOUND OR u.shipyard_work_remaining IS NULL THEN RETURN; END IF;
 SELECT current_tick INTO t FROM worlds WHERE id=u.world_id;
 work:=shipyard_work_left(u.shipyard_work_remaining,u.shipyard_work_rate,u.shipyard_work_calc_tick,t);
 rate:=0;
 IF u.category='naval' AND u.status IN ('forming','repairing') THEN
  rate:=shipyard_workers(u.settlement_id,u.owner_id)*60/shipyard_sharing(u.settlement_id,u.owner_id);
 END IF;
 IF u.status NOT IN ('forming','repairing') OR u.category<>'naval' THEN
  UPDATE units SET shipyard_work_remaining=NULL,shipyard_work_running=false,shipyard_work_rate=0,build_complete_at=NULL,shipyard_sped=false WHERE id=ship;
  UPDATE scheduled_events SET processed_at=now() WHERE event_type='ShipyardWorkComplete' AND payload->>'unit_id'=ship::text AND processed_at IS NULL;
  RETURN;
 END IF;
 scale:=u.status='forming' AND NOT u.shipyard_sped AND work>0 AND building_blessed(u.settlement_id,t);
 IF t=u.shipyard_work_calc_tick AND rate=u.shipyard_work_rate AND NOT scale THEN RETURN; END IF;
 IF scale THEN work:=blessed_build_work(work,rate); END IF;
 rev:=u.shipyard_work_revision+1;
 UPDATE units SET shipyard_work_remaining=work,shipyard_work_calc_tick=t,shipyard_work_running=(rate>0),shipyard_work_rate=rate,shipyard_work_revision=rev,build_complete_at=NULL,shipyard_sped=(u.shipyard_sped OR scale) WHERE id=ship;
 UPDATE scheduled_events SET processed_at=now() WHERE event_type='ShipyardWorkComplete' AND payload->>'unit_id'=ship::text AND processed_at IS NULL;
 IF rate>0 THEN
  INSERT INTO scheduled_events(world_id,event_type,payload,process_after,due_tick,priority)
  VALUES(u.world_id,'ShipyardWorkComplete',jsonb_build_object('unit_id',ship,'revision',rev),now(),t+(work+rate-1)/rate,10);
 END IF;
END $$;

CREATE FUNCTION sync_shipyard_city(city uuid) RETURNS void LANGUAGE plpgsql AS $$
DECLARE ship uuid;
BEGIN
 IF city IS NULL THEN RETURN; END IF;
 FOR ship IN SELECT id FROM units WHERE settlement_id=city AND shipyard_work_remaining IS NOT NULL ORDER BY id LOOP
  PERFORM sync_shipyard_work(ship);
 END LOOP;
END $$;

CREATE OR REPLACE FUNCTION shipyard_unit_changed() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 PERFORM sync_shipyard_work(NEW.id);
 IF NEW.category='naval' THEN
  PERFORM sync_shipyard_city(NEW.settlement_id);
  IF OLD.settlement_id IS DISTINCT FROM NEW.settlement_id THEN PERFORM sync_shipyard_city(OLD.settlement_id); END IF;
 END IF;
 RETURN NULL;
END $$;

-- Jobs in flight: settle under the OLD rate to now, store in 60ths, keep the finish tick. A city with
-- one ship gets the same rate*60 and the same timer; a city with several is then shared.
-- BEGIN CONVERT
DO $$
DECLARE u record; t integer; city uuid;
BEGIN
 FOR u IN SELECT id,world_id,shipyard_work_remaining AS r,shipyard_work_rate AS rt,shipyard_work_calc_tick AS c FROM units WHERE shipyard_work_remaining IS NOT NULL ORDER BY id LOOP
  SELECT current_tick INTO t FROM worlds WHERE id=u.world_id;
  UPDATE units SET shipyard_work_remaining=shipyard_work_left(u.r,u.rt,u.c,t)*60,shipyard_work_calc_tick=t,shipyard_work_rate=u.rt*60 WHERE id=u.id;
 END LOOP;
 FOR city IN SELECT DISTINCT settlement_id FROM units WHERE shipyard_work_remaining IS NOT NULL AND settlement_id IS NOT NULL ORDER BY 1 LOOP
  PERFORM sync_shipyard_city(city);
 END LOOP;
END $$;
-- END CONVERT
