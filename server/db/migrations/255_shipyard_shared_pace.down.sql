-- Back to whole worker-ticks and one full rate per ship. Work is settled to now and rounded UP to a
-- worker-tick (never less work than was left); the rate is re-derived by mig 251's sync.
DO $$
DECLARE u record; t integer;
BEGIN
 FOR u IN SELECT id,world_id,shipyard_work_remaining AS r,shipyard_work_rate AS rt,shipyard_work_calc_tick AS c FROM units WHERE shipyard_work_remaining IS NOT NULL ORDER BY id LOOP
  SELECT current_tick INTO t FROM worlds WHERE id=u.world_id;
  UPDATE units SET shipyard_work_remaining=(shipyard_work_left(u.r,u.rt,u.c,t)+59)/60,shipyard_work_calc_tick=NULL,shipyard_work_rate=0 WHERE id=u.id;
 END LOOP;
END $$;
ALTER TABLE units DROP CONSTRAINT units_shipyard_work_rate_check;
ALTER TABLE units ADD CONSTRAINT units_shipyard_work_rate_check CHECK(shipyard_work_rate BETWEEN 0 AND 3);
CREATE OR REPLACE FUNCTION sync_shipyard_work(ship uuid) RETURNS void LANGUAGE plpgsql AS $$
DECLARE u units%ROWTYPE; t integer; work integer; rate integer; rev integer; scale boolean;
BEGIN
 SELECT * INTO u FROM units WHERE id=ship FOR UPDATE;
 IF NOT FOUND OR u.shipyard_work_remaining IS NULL THEN RETURN; END IF;
 SELECT current_tick INTO t FROM worlds WHERE id=u.world_id;
 work:=shipyard_work_left(u.shipyard_work_remaining,u.shipyard_work_rate,u.shipyard_work_calc_tick,t);
 rate:=0;
 IF u.category='naval' AND u.status IN ('forming','repairing') THEN
  SELECT least(3,count(*)) INTO rate FROM settlements s
  JOIN buildings b ON b.settlement_id=s.id AND b.building_type='shipyard' AND b.level>=1
  JOIN settlement_placement p ON p.settlement_id=s.id AND p.target_kind='building' AND p.building_type='shipyard' AND p.good_key='shipbuilding'
  WHERE s.id=u.settlement_id AND s.owner_id=u.owner_id AND s.state='active' AND s.population>=100;
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

CREATE OR REPLACE FUNCTION shipyard_unit_changed() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN PERFORM sync_shipyard_work(NEW.id); RETURN NULL; END $$;
DO $$
DECLARE s uuid;
BEGIN
 FOR s IN SELECT id FROM units WHERE shipyard_work_remaining IS NOT NULL ORDER BY id LOOP PERFORM sync_shipyard_work(s); END LOOP;
END $$;
DROP FUNCTION sync_shipyard_city(uuid);
DROP FUNCTION shipyard_sharing(uuid,uuid);
DROP FUNCTION shipyard_workers(uuid,uuid);
