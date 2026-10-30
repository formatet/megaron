-- One StorageOverflow notice per ongoing production loss (Claude 2026-10-30, before the
-- alfa reset): the 198 rule only treated a loss on the PREVIOUS tick as ongoing, so a
-- store that refills to its cap every 5 ticks notified every 5 ticks. Events unchanged.
CREATE FUNCTION storage_overflow_quiet_ticks() RETURNS INT LANGUAGE sql IMMUTABLE AS $$ SELECT 24 $$;

CREATE OR REPLACE FUNCTION storage_capped(p_sid UUID,p_good TEXT,p_available DOUBLE PRECISION,p_cap DOUBLE PRECISION,p_reason TEXT)
RETURNS DOUBLE PRECISION LANGUAGE plpgsql VOLATILE AS $$
DECLARE lost DOUBLE PRECISION; wid UUID; owner UUID; pname TEXT; pid UUID;
        tick INT; body JSONB; eid BIGINT; nid UUID;
BEGIN
    lost:=GREATEST(0,p_available-p_cap);
    IF lost>0 THEN
        SELECT s.world_id,s.owner_id,s.name,s.province_id,w.current_tick
        INTO wid,owner,pname,pid,tick FROM settlements s JOIN worlds w ON w.id=s.world_id WHERE s.id=p_sid;
        IF wid IS NULL THEN RAISE EXCEPTION 'storage outcome has no settlement %',p_sid; END IF;
        body:=jsonb_build_object('settlement_id',p_sid,'province_id',pid,'name',pname,
            'good_key',p_good,'lost_quantity',lost,'cap',p_cap,'reason',p_reason,
            'world_tick',tick,'player_id',owner);
        INSERT INTO events(stream_id,stream_type,event_type,payload,world_id)
        VALUES(p_sid,'province','StorageOverflow',body,wid) RETURNING id INTO eid;
        -- Production into a full store is lost every tick it stays full; tell the
        -- owner when it STARTS, not every day (live had 193 silver notices on 193
        -- ticks). "Starts" = no production overflow of this good within
        -- storage_overflow_quiet_ticks() — a store that refills to its cap every few
        -- ticks (fish, 5-tick gaps: 465 notices) is one ongoing loss, not many.
        -- The event above is still written each time — it is the outcome.
        IF owner IS NOT NULL AND (p_reason <> 'production' OR NOT EXISTS(
               SELECT 1 FROM events e WHERE e.stream_id=p_sid AND e.id<>eid
                  AND e.event_type='StorageOverflow' AND e.payload->>'good_key'=p_good
                  AND e.payload->>'reason'='production' AND (e.payload->>'world_tick')::int > tick-storage_overflow_quiet_ticks())) THEN
            INSERT INTO notifications(world_id,player_id,kind,level,body_json,world_tick)
            VALUES(wid,owner,'StorageOverflow',2,body,tick) RETURNING id INTO nid;
            INSERT INTO scheduled_events(world_id,event_type,payload,process_after,due_tick,priority)
            VALUES(wid,'StorageOverflowDelivery',jsonb_build_object('notification_id',nid,'event_id',eid),now(),tick,50);
        END IF;
    END IF;
    RETURN LEAST(p_cap,GREATEST(0,p_available));
END $$;
