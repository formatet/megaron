-- More hands = faster work (Timothy 2026-11-01, megaron_byggare_verkstad.md).
--   * Shipyard: work rate = placed shipbuilding workers, capped at 3 (was a boolean
--     "somebody stands there" = exactly 1 unit of work per tick). Slots per level become 1/2/3.
--   * Workshop: new building, builders (role 'building') speed up construction 1x/2x/3x.
--     No builders (or no workshop) = base pace 1x, so the first Workshop can be built.
--   * Build queue: ONE active build per city, the rest wait (3 rows at most, enforced by the
--     handler). Same remaining / rate / calc_tick model as shipyard work: the BuildComplete
--     event is re-issued with a revision whenever the rate changes.

-- ── Roles ──────────────────────────────────────────────────────────────────
INSERT INTO goods(key,name,tier,category,base_value,weight,status)
VALUES('building','Construction work','manufactured','bulk',1,0,'parked');

-- ── Shipyard: rate instead of running ───────────────────────────────────────
ALTER TABLE units ADD COLUMN shipyard_work_rate integer NOT NULL DEFAULT 0 CHECK(shipyard_work_rate BETWEEN 0 AND 3);
UPDATE units SET shipyard_work_rate=CASE WHEN shipyard_work_running THEN 1 ELSE 0 END WHERE shipyard_work_remaining IS NOT NULL;

DROP FUNCTION shipyard_work_left(integer,boolean,integer,integer);
CREATE FUNCTION shipyard_work_left(work integer, rate integer, calc integer, at_tick integer)
RETURNS integer LANGUAGE sql IMMUTABLE AS $$ SELECT greatest(0,work-greatest(0,at_tick-calc)*rate) $$;

-- Settle the OLD saved rate before deriving the new one (unchanged principle from mig 204).
-- shipyard_work_running is kept as (rate>0) so readers of the boolean stay correct.
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

-- Slots per level become 1/2/3 (was 3/6/10). Workers above the new ceiling become free
-- citizens again (lowest ordinals stay). Triggers are off for this one DELETE so a job in
-- flight keeps its saved rate and finishes on the tick the old rule promised; the next
-- staffing change re-derives the rate.
ALTER TABLE settlement_placement DISABLE TRIGGER shipyard_placement_change;
DELETE FROM settlement_placement p USING (
 SELECT q.id FROM (
  SELECT p2.id, b.level, row_number() OVER (PARTITION BY p2.settlement_id ORDER BY p2.gubbe_ordinal) AS rn
  FROM settlement_placement p2 JOIN buildings b ON b.settlement_id=p2.settlement_id AND b.building_type='shipyard'
  WHERE p2.target_kind='building' AND p2.building_type='shipyard') q
 WHERE q.rn>q.level) x
WHERE p.id=x.id;
ALTER TABLE settlement_placement ENABLE TRIGGER shipyard_placement_change;

-- ── Build queue: one active, the rest wait ──────────────────────────────────
ALTER TABLE build_queue ADD COLUMN work_remaining integer CHECK(work_remaining>=0),
 ADD COLUMN work_calc_tick integer,
 ADD COLUMN work_rate integer NOT NULL DEFAULT 0 CHECK(work_rate BETWEEN 0 AND 3),
 ADD COLUMN work_revision integer NOT NULL DEFAULT 0;

-- A started row has work_calc_tick set. Work left: settled under the SAVED rate.
CREATE FUNCTION build_work_left(work integer, rate integer, calc integer, at_tick integer)
RETURNS integer LANGUAGE sql IMMUTABLE AS $$ SELECT CASE WHEN calc IS NULL THEN work ELSE greatest(0,work-greatest(0,at_tick-calc)*rate) END $$;

-- Builders in the city's Workshop (1x/2x/3x); no workshop or nobody placed = base pace 1.
CREATE FUNCTION build_rate(city uuid) RETURNS integer LANGUAGE sql STABLE AS $$
 SELECT greatest(1,least(3,count(*)))::integer FROM settlement_placement p
 JOIN buildings b ON b.settlement_id=p.settlement_id AND b.building_type='workshop' AND b.level>=1
 WHERE p.settlement_id=city AND p.target_kind='building' AND p.building_type='workshop' AND p.good_key='building' $$;

-- The one owner of the queue's clock. Settles the active row under its OLD rate, derives the
-- new rate, re-issues the completion event (revision-stamped), and starts the oldest waiting
-- row when nothing is active. All writers (placements, buildings, enqueue, complete, cancel)
-- call it in their own transaction.
CREATE FUNCTION sync_build_queue(city uuid) RETURNS void LANGUAGE plpgsql AS $$
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

CREATE FUNCTION build_city_changed() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE city uuid;
BEGIN
 IF TG_TABLE_NAME='buildings' THEN
  IF TG_OP='DELETE' THEN
   IF OLD.building_type<>'workshop' THEN RETURN NULL; END IF;
   DELETE FROM settlement_placement WHERE settlement_id=OLD.settlement_id AND building_type='workshop';
   city:=OLD.settlement_id;
  ELSE
   IF NEW.building_type<>'workshop' THEN RETURN NULL; END IF;
   city:=NEW.settlement_id;
  END IF;
 ELSE
  IF TG_OP='DELETE' THEN
   IF OLD.building_type IS DISTINCT FROM 'workshop' THEN RETURN NULL; END IF;
   city:=OLD.settlement_id;
  ELSE
   IF NEW.building_type IS DISTINCT FROM 'workshop' THEN RETURN NULL; END IF;
   city:=NEW.settlement_id;
  END IF;
 END IF;
 PERFORM sync_build_queue(city);
 RETURN NULL;
END $$;
CREATE TRIGGER build_placement_change AFTER INSERT OR UPDATE OR DELETE ON settlement_placement FOR EACH ROW EXECUTE FUNCTION build_city_changed();
CREATE TRIGGER build_building_change AFTER INSERT OR UPDATE OR DELETE ON buildings FOR EACH ROW EXECUTE FUNCTION build_city_changed();

-- Rows queued before this migration: capture the OLD deadline as work left (never lost,
-- never paid twice), retire the old fixed-tick events, then let sync start each city's
-- oldest row at base pace 1 -- it finishes on the tick the old rule gave it. A second
-- row that used to run in parallel now waits with its remaining work preserved.
UPDATE build_queue q SET work_remaining=greatest(0,coalesce(
  (SELECT min(e.due_tick) FROM scheduled_events e WHERE e.event_type='BuildComplete' AND e.payload->>'build_queue_id'=q.id::text AND e.processed_at IS NULL AND e.failed_at IS NULL),
  (SELECT current_tick FROM worlds w WHERE w.id=q.world_id))-(SELECT current_tick FROM worlds w WHERE w.id=q.world_id));
UPDATE scheduled_events SET processed_at=now() WHERE event_type='BuildComplete' AND processed_at IS NULL AND failed_at IS NULL;
SELECT sync_build_queue(sid) FROM (SELECT DISTINCT settlement_id AS sid FROM build_queue) s;
