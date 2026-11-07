-- The Builder's Blessing (bönesystemet v3 steg 5a, Timothy 2026-11-07): what is left of a build takes
-- three quarters of the time, ONCE per build (the marker), and a build that starts while the
-- blessing lasts gets the same cut from its start. One code path: the two sync functions that own the
-- queue's clock (mig 245) do the cut themselves, so the cast only has to call them; the old
-- BuildComplete / ShipyardWorkComplete is retired by the revision they already bump.
--   build_queue.sped, units.shipyard_sped : this build / ship job has had its cut.
--   A ship is cut only while 'forming' (building a ship); a repair is not construction.
ALTER TABLE build_queue ADD COLUMN sped boolean NOT NULL DEFAULT false;
ALTER TABLE units ADD COLUMN shipyard_sped boolean NOT NULL DEFAULT false;

-- Is a live Builder's Blessing laid on this city (its owner's city blessing, or the owner's realm)?
CREATE FUNCTION building_blessed(city uuid, at_tick integer) RETURNS boolean LANGUAGE sql STABLE AS $$
 SELECT EXISTS(SELECT 1 FROM prayer_blessings b JOIN settlements s ON s.id=city AND s.owner_id=b.owner_id
  WHERE b.effect='blessed_building' AND b.from_tick<=at_tick AND b.until_tick>at_tick
    AND (b.scope='realm' OR (b.scope='city' AND b.settlement_id=city))) $$;

-- Work left after the cut: T = days left at this rate, T' = max(1, ceil(3T/4)), stored as the least
-- work that still takes T' days, so the finish is exactly now + T' (never before now + 1).
CREATE FUNCTION blessed_build_work(work integer, rate integer) RETURNS integer LANGUAGE sql IMMUTABLE AS $$
 SELECT least(work, (greatest(1,(3*((work+greatest(rate,1)-1)/greatest(rate,1))+3)/4)-1)*greatest(rate,1)+1) $$;

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

CREATE OR REPLACE FUNCTION sync_build_queue(city uuid) RETURNS void LANGUAGE plpgsql AS $$
DECLARE r build_queue%ROWTYPE; t integer; work integer; rate integer; rev integer; scale boolean;
BEGIN
 SELECT * INTO r FROM build_queue WHERE settlement_id=city AND work_calc_tick IS NOT NULL ORDER BY created_at,id LIMIT 1 FOR UPDATE;
 IF NOT FOUND THEN
  SELECT * INTO r FROM build_queue WHERE settlement_id=city ORDER BY created_at,id LIMIT 1 FOR UPDATE;
  IF NOT FOUND THEN RETURN; END IF;
 END IF;
 SELECT current_tick INTO t FROM worlds WHERE id=r.world_id;
 work:=build_work_left(r.work_remaining,r.work_rate,r.work_calc_tick,t);
 rate:=build_rate(city);
 scale:=NOT r.sped AND work>0 AND building_blessed(city,t);
 IF r.work_calc_tick IS NOT NULL AND t=r.work_calc_tick AND rate=r.work_rate AND NOT scale THEN RETURN; END IF;
 IF scale THEN work:=blessed_build_work(work,rate); END IF;
 rev:=r.work_revision+1;
 UPDATE build_queue SET work_remaining=work,work_calc_tick=t,work_rate=rate,work_revision=rev,sped=(r.sped OR scale) WHERE id=r.id;
 UPDATE scheduled_events SET processed_at=now() WHERE event_type='BuildComplete' AND payload->>'build_queue_id'=r.id::text AND processed_at IS NULL;
 INSERT INTO scheduled_events(world_id,event_type,payload,process_after,due_tick,priority)
 VALUES(r.world_id,'BuildComplete',jsonb_build_object('settlement_id',city,'build_queue_id',r.id,'building_type',r.building_type,'hex_q',r.hex_q,'hex_r',r.hex_r,'revision',rev),now(),t+(work+rate-1)/rate,10);
END $$;
