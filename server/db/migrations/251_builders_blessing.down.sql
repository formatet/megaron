-- Back to mig 245's sync functions.
CREATE OR REPLACE FUNCTION sync_shipyard_work(ship uuid) RETURNS void LANGUAGE plpgsql AS $$
DECLARE u units%ROWTYPE; t integer; work integer; rate integer; rev integer;
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
  UPDATE units SET shipyard_work_remaining=NULL,shipyard_work_running=false,shipyard_work_rate=0,build_complete_at=NULL WHERE id=ship;
  UPDATE scheduled_events SET processed_at=now() WHERE event_type='ShipyardWorkComplete' AND payload->>'unit_id'=ship::text AND processed_at IS NULL;
  RETURN;
 END IF;
 IF t=u.shipyard_work_calc_tick AND rate=u.shipyard_work_rate THEN RETURN; END IF;
 rev:=u.shipyard_work_revision+1;
 UPDATE units SET shipyard_work_remaining=work,shipyard_work_calc_tick=t,shipyard_work_running=(rate>0),shipyard_work_rate=rate,shipyard_work_revision=rev,build_complete_at=NULL WHERE id=ship;
 UPDATE scheduled_events SET processed_at=now() WHERE event_type='ShipyardWorkComplete' AND payload->>'unit_id'=ship::text AND processed_at IS NULL;
 IF rate>0 THEN
  INSERT INTO scheduled_events(world_id,event_type,payload,process_after,due_tick,priority)
  VALUES(u.world_id,'ShipyardWorkComplete',jsonb_build_object('unit_id',ship,'revision',rev),now(),t+(work+rate-1)/rate,10);
 END IF;
END $$;

CREATE OR REPLACE FUNCTION sync_build_queue(city uuid) RETURNS void LANGUAGE plpgsql AS $$
DECLARE r build_queue%ROWTYPE; t integer; work integer; rate integer; rev integer;
BEGIN
 SELECT * INTO r FROM build_queue WHERE settlement_id=city AND work_calc_tick IS NOT NULL ORDER BY created_at,id LIMIT 1 FOR UPDATE;
 IF NOT FOUND THEN
  SELECT * INTO r FROM build_queue WHERE settlement_id=city ORDER BY created_at,id LIMIT 1 FOR UPDATE;
  IF NOT FOUND THEN RETURN; END IF;
 END IF;
 SELECT current_tick INTO t FROM worlds WHERE id=r.world_id;
 work:=build_work_left(r.work_remaining,r.work_rate,r.work_calc_tick,t);
 rate:=build_rate(city);
 IF r.work_calc_tick IS NOT NULL AND t=r.work_calc_tick AND rate=r.work_rate THEN RETURN; END IF;
 rev:=r.work_revision+1;
 UPDATE build_queue SET work_remaining=work,work_calc_tick=t,work_rate=rate,work_revision=rev WHERE id=r.id;
 UPDATE scheduled_events SET processed_at=now() WHERE event_type='BuildComplete' AND payload->>'build_queue_id'=r.id::text AND processed_at IS NULL;
 INSERT INTO scheduled_events(world_id,event_type,payload,process_after,due_tick,priority)
 VALUES(r.world_id,'BuildComplete',jsonb_build_object('settlement_id',city,'build_queue_id',r.id,'building_type',r.building_type,'hex_q',r.hex_q,'hex_r',r.hex_r,'revision',rev),now(),t+(work+rate-1)/rate,10);
END $$;

DROP FUNCTION blessed_build_work(integer,integer);
DROP FUNCTION building_blessed(uuid,integer);
ALTER TABLE units DROP COLUMN shipyard_sped;
ALTER TABLE build_queue DROP COLUMN sped;
