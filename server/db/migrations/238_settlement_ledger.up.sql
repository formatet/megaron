-- Bokförd dagsräkning (ekonomins sanning, Timothy 2026-11-04). Sanningen är lagret;
-- ledgern förklarar varje förändring av settlement_goods.amount med en kategori.
--
-- Periodbegreppet: en post får period = current_world_tick(), OM stadens dagsavslut
-- (settlement_ledger_close) för den ticken ännu inte tagits, annars tick+1. Då är
-- Δ(period N) = closing(N) − closing(N−1) exakt lika med summan av posterna i period N.
CREATE TABLE settlement_ledger (
    settlement_id UUID NOT NULL REFERENCES settlements(id) ON DELETE CASCADE,
    tick          INT  NOT NULL,
    good_key      TEXT NOT NULL,
    category      TEXT NOT NULL,
    amount        DOUBLE PRECISION NOT NULL,
    PRIMARY KEY (settlement_id, tick, good_key, category)
);
CREATE INDEX settlement_ledger_tick_idx ON settlement_ledger (tick);

CREATE TABLE settlement_ledger_close (
    settlement_id UUID NOT NULL REFERENCES settlements(id) ON DELETE CASCADE,
    tick          INT  NOT NULL,
    good_key      TEXT NOT NULL,
    amount        DOUBLE PRECISION NOT NULL,
    PRIMARY KEY (settlement_id, tick, good_key)
);
CREATE INDEX settlement_ledger_close_tick_idx ON settlement_ledger_close (tick);

CREATE FUNCTION ledger_post(p_sid UUID, p_good TEXT, p_cat TEXT, p_amount DOUBLE PRECISION)
RETURNS VOID LANGUAGE plpgsql AS $$
DECLARE cur INT := current_world_tick(); period INT;
BEGIN
    IF cur IS NULL OR p_amount IS NULL OR abs(p_amount) < 1e-9 THEN RETURN; END IF;
    period := cur + CASE WHEN EXISTS(SELECT 1 FROM settlement_ledger_close c WHERE c.settlement_id = p_sid AND c.tick = cur) THEN 1 ELSE 0 END;
    INSERT INTO settlement_ledger(settlement_id, tick, good_key, category, amount)
    VALUES (p_sid, period, p_good, p_cat, p_amount)
    ON CONFLICT (settlement_id, tick, good_key, category) DO UPDATE SET amount = settlement_ledger.amount + EXCLUDED.amount;
END $$;

-- storage_capped: förlorat överskott läggs i ett tx-lokalt konto som ledger-triggern
-- tömmer på raden som just skrevs (BEFORE ROW-trigger ⇒ raderna hanteras en i taget).
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

-- credit_settlement_good stämplar sin egen reason på posten.
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

-- Triggern: varje ändring av amount förklaras i samma transaktion.
--   production = rate × förflutna ticks sedan calc_tick (lazy-ackrualen)
--   overflow   = −(det storage_capped klippte)
--   resten     = kategorin i megaron.ledger_reason, annars 'unlabelled' (= "Other")
CREATE FUNCTION settlement_goods_ledger() RETURNS TRIGGER LANGUAGE plpgsql AS $$
DECLARE cur INT := current_world_tick(); accrual DOUBLE PRECISION := 0; ovf DOUBLE PRECISION; rest DOUBLE PRECISION;
        reason TEXT; base DOUBLE PRECISION := 0;
BEGIN
    IF cur IS NULL THEN RETURN NEW; END IF;
    ovf := COALESCE(NULLIF(current_setting('megaron.ledger_overflow', true), ''), '0')::float8;
    IF ovf <> 0 THEN PERFORM set_config('megaron.ledger_overflow', '0', true); END IF;
    reason := COALESCE(NULLIF(current_setting('megaron.ledger_reason', true), ''), 'unlabelled');
    IF TG_OP = 'UPDATE' THEN
        accrual := OLD.rate * GREATEST(0, cur - OLD.calc_tick);
        base := OLD.amount;
    END IF;
    rest := NEW.amount - base - accrual + ovf;
    PERFORM ledger_post(NEW.settlement_id, NEW.good_key, 'production', accrual);
    PERFORM ledger_post(NEW.settlement_id, NEW.good_key, 'overflow', -ovf);
    PERFORM ledger_post(NEW.settlement_id, NEW.good_key, reason, rest);
    RETURN NEW;
END $$;

CREATE TRIGGER settlement_goods_ledger_upd BEFORE UPDATE ON settlement_goods
    FOR EACH ROW WHEN (OLD.amount IS DISTINCT FROM NEW.amount OR OLD.calc_tick IS DISTINCT FROM NEW.calc_tick)
    EXECUTE FUNCTION settlement_goods_ledger();
CREATE TRIGGER settlement_goods_ledger_ins AFTER INSERT ON settlement_goods
    FOR EACH ROW WHEN (NEW.amount <> 0) EXECUTE FUNCTION settlement_goods_ledger();
