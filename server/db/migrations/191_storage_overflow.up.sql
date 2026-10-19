-- Raise ceilings only. Preserve every balance, rate and production anchor.
UPDATE settlement_goods g SET cap=GREATEST(g.cap,960,g.amount)
FROM settlements s WHERE s.id=g.settlement_id AND g.good_key='silver' AND NOT s.is_capital;

-- Called only inside a goods mutation under its row lock. Outcomes, inbox and
-- live-delivery job commit or roll back with the credit/production realization.
CREATE FUNCTION storage_capped(p_sid UUID,p_good TEXT,p_available DOUBLE PRECISION,p_cap DOUBLE PRECISION,p_reason TEXT)
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
        IF owner IS NOT NULL THEN
            INSERT INTO notifications(world_id,player_id,kind,level,body_json,world_tick)
            VALUES(wid,owner,'StorageOverflow',2,body,tick) RETURNING id INTO nid;
            INSERT INTO scheduled_events(world_id,event_type,payload,process_after,due_tick,priority)
            VALUES(wid,'StorageOverflowDelivery',jsonb_build_object('notification_id',nid,'event_id',eid),now(),tick,50);
        END IF;
    END IF;
    RETURN LEAST(p_cap,GREATEST(0,p_available));
END $$;

CREATE FUNCTION credit_settlement_good(p_sid UUID,p_good TEXT,p_qty DOUBLE PRECISION,p_reason TEXT,p_default_cap DOUBLE PRECISION DEFAULT 1000000)
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
