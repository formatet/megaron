DROP TRIGGER IF EXISTS settlement_goods_ledger_upd ON settlement_goods;
DROP TRIGGER IF EXISTS settlement_goods_ledger_ins ON settlement_goods;
DROP FUNCTION IF EXISTS settlement_goods_ledger();
DROP TABLE IF EXISTS settlement_ledger_close;
DROP TABLE IF EXISTS settlement_ledger;
DROP FUNCTION IF EXISTS ledger_post(UUID,TEXT,TEXT,DOUBLE PRECISION);

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

CREATE OR REPLACE FUNCTION credit_settlement_good(p_sid UUID,p_good TEXT,p_qty DOUBLE PRECISION,p_reason TEXT,p_default_cap DOUBLE PRECISION DEFAULT 1000000)
RETURNS DOUBLE PRECISION LANGUAGE plpgsql VOLATILE AS $$
DECLARE g settlement_goods%ROWTYPE; stock DOUBLE PRECISION; result DOUBLE PRECISION;
BEGIN
    IF p_qty<0 THEN RAISE EXCEPTION 'negative storage credit'; END IF;
    INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick)
    VALUES(p_sid,p_good,0,0,p_default_cap,current_world_tick()) ON CONFLICT DO NOTHING;
    SELECT * INTO STRICT g FROM settlement_goods WHERE settlement_id=p_sid AND good_key=p_good FOR UPDATE;
    stock:=storage_capped(p_sid,p_good,settled(g.amount,g.rate,g.calc_tick),g.cap,'production');
    result:=storage_capped(p_sid,p_good,stock+p_qty,g.cap,p_reason);
    UPDATE settlement_goods SET amount=result,calc_tick=current_world_tick()
    WHERE settlement_id=p_sid AND good_key=p_good;
    RETURN result-stock;
END $$;
