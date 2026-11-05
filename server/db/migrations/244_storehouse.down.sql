-- Back to the mig 238 / 179 bodies first, then drop what they no longer use.
CREATE OR REPLACE FUNCTION storage_capped(p_sid UUID,p_good TEXT,p_available DOUBLE PRECISION,p_cap DOUBLE PRECISION,p_reason TEXT)
RETURNS DOUBLE PRECISION LANGUAGE plpgsql VOLATILE AS $$
DECLARE lost DOUBLE PRECISION; wid UUID; owner UUID; pname TEXT; pid UUID;
        tick INT; body JSONB; eid BIGINT; nid UUID;
BEGIN
    lost:=GREATEST(0,p_available-p_cap);
    IF lost>0 THEN
        PERFORM set_config('megaron.ledger_overflow', (COALESCE(NULLIF(current_setting('megaron.ledger_overflow',true),''),'0')::float8 + lost)::text, true);
        SELECT s.world_id,s.owner_id,s.name,s.province_id,w.current_tick
        INTO wid,owner,pname,pid,tick FROM settlements s JOIN worlds w ON w.id=s.world_id WHERE s.id=p_sid;
        IF wid IS NULL THEN RAISE EXCEPTION 'storage outcome has no settlement %',p_sid; END IF;
        body:=jsonb_build_object('settlement_id',p_sid,'province_id',pid,'name',pname,
            'good_key',p_good,'lost_quantity',lost,'cap',p_cap,'reason',p_reason,
            'world_tick',tick,'player_id',owner);
        INSERT INTO events(stream_id,stream_type,event_type,payload,world_id)
        VALUES(p_sid,'province','StorageOverflow',body,wid) RETURNING id INTO eid;
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
    PERFORM set_config('megaron.ledger_reason', p_reason, true);
    UPDATE settlement_goods SET amount=result,calc_tick=current_world_tick()
    WHERE settlement_id=p_sid AND good_key=p_good;
    PERFORM set_config('megaron.ledger_reason', '', true);
    RETURN result-stock;
END $$;

CREATE OR REPLACE FUNCTION record_silver_mining_credit() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE credited DOUBLE PRECISION; wid UUID;
BEGIN
    IF OLD.good_key <> 'silver' OR OLD.rate <= 0 OR NEW.calc_tick <= OLD.calc_tick THEN
        RETURN NEW;
    END IF;
    -- Realized production before this mutation, clipped at the OLD cap. Transfers
    -- and payments in NEW.amount are not mining. A full warehouse produces zero.
    credited := GREATEST(0,
        LEAST(OLD.cap, GREATEST(0, OLD.amount + OLD.rate * (NEW.calc_tick - OLD.calc_tick)))
        - LEAST(OLD.cap, GREATEST(0, OLD.amount)));
    IF credited > 0 THEN
        SELECT world_id INTO wid FROM settlements WHERE id = OLD.settlement_id;
        INSERT INTO silver_mining_totals(settlement_id, world_id, credited_silver) VALUES(OLD.settlement_id, wid, credited)
        ON CONFLICT (settlement_id) DO UPDATE SET
            credited_silver = silver_mining_totals.credited_silver + EXCLUDED.credited_silver;
    END IF;
    RETURN NEW;
END
$$;

DROP TRIGGER settlement_goods_clip_storehouse ON settlement_goods;
DROP FUNCTION settlement_goods_storehouse_clip();
DROP FUNCTION storehouse_effective(UUID, TEXT, DOUBLE PRECISION);
DROP FUNCTION storehouse_stored_good(TEXT);
DROP FUNCTION storehouse_cap(UUID);
DROP FUNCTION settlement_storehouse_level(UUID);
DROP FUNCTION storehouse_mult(INT);
DROP FUNCTION storehouse_base();
DELETE FROM buildings WHERE building_type = 'storehouse';
