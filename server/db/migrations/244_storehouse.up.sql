-- Storehouse (Timothy 2026-10-28/29): ONE shared store for every good except food
-- (silo_stored_good), the herd (livestock), the parked horses and cult. Ceiling
-- 500 with no building, x2/x5/x10 at Storehouse level 1/2/3. Numbers owned by
-- province.StorehouseBase / StorehouseStorageMult (TestStorehouseSQLMirror pins them).
--
-- Design (docs/reviews/forradet/README.md): the ceiling is NOT written into any row.
-- It is computed live from the building level, so build / level / demolish / conquest
-- need no re-capping trigger, and stock above the ceiling is never touched: the
-- ceiling only decides how much NEW production or delivery is admitted.
--   * storehouse_effective() is the pure read: per member, base (what is held, a
--     falling stock already netted) + the admitted share of its pending accrual.
--     When the pending accruals of all members together exceed the room they are
--     admitted IN PROPORTION to their accrual (Timothy 2026-10-29: "ingen vara går
--     före"). The same function answers every writer, so loop order is irrelevant.
--   * storage_capped() — already the single gate of production / refining — takes
--     the effective cap for pool goods. settled() itself is untouched.
--   * a BEFORE UPDATE trigger clips the accrual realized by writers that do not go
--     through storage_capped (payments that realize with settled()).
--   * credit_settlement_good takes a per-city advisory lock first and admits at most
--     the remaining room; excess is lost with a StorageOverflow outcome (as the silo).
CREATE FUNCTION storehouse_base() RETURNS INT LANGUAGE sql IMMUTABLE AS $$ SELECT 500 $$;

CREATE FUNCTION storehouse_mult(p_level INT) RETURNS INT LANGUAGE sql IMMUTABLE AS $$
    SELECT CASE WHEN p_level >= 3 THEN 10 WHEN p_level = 2 THEN 5 WHEN p_level = 1 THEN 2 ELSE 1 END
$$;

CREATE FUNCTION settlement_storehouse_level(p_sid UUID) RETURNS INT LANGUAGE sql STABLE AS $$
    SELECT COALESCE(MAX(level),0)::int FROM buildings WHERE settlement_id=p_sid AND building_type='storehouse'
$$;

CREATE FUNCTION storehouse_cap(p_sid UUID) RETURNS DOUBLE PRECISION LANGUAGE sql STABLE AS $$
    SELECT (storehouse_base() * storehouse_mult(settlement_storehouse_level(p_sid)))::double precision
$$;

-- Everything the silo (food), the pen (livestock) and the temples (cult) do not hold.
-- Horses are parked outside the Storehouse until the horse model is decided.
CREATE FUNCTION storehouse_stored_good(p_good TEXT) RETURNS BOOLEAN LANGUAGE sql IMMUTABLE AS $$
    SELECT NOT silo_stored_good(p_good) AND p_good NOT IN ('livestock','horses','cult')
$$;

-- Pure read at the current tick. p_good/p_avail override one member's realizable
-- amount (a writer asking "what if THIS row realizes to p_avail").
CREATE FUNCTION storehouse_effective(p_sid UUID, p_good TEXT DEFAULT NULL, p_avail DOUBLE PRECISION DEFAULT NULL)
RETURNS TABLE(good_key TEXT, amount DOUBLE PRECISION, base DOUBLE PRECISION, accrual DOUBLE PRECISION, admitted DOUBLE PRECISION)
LANGUAGE sql STABLE AS $$
    WITH m AS (
        SELECT g.good_key, g.amount AS a,
               CASE WHEN p_good IS NOT NULL AND g.good_key = p_good THEN p_avail
                    ELSE settled(g.amount, g.rate, g.calc_tick) END AS s
          FROM settlement_goods g
         WHERE g.settlement_id = p_sid AND storehouse_stored_good(g.good_key)
    ), b AS (
        SELECT m.good_key, GREATEST(0, LEAST(m.a, m.s)) AS base, GREATEST(0, m.s - m.a) AS acc FROM m
    ), t AS (
        SELECT GREATEST(0, storehouse_cap(p_sid) - COALESCE(sum(b.base),0)) AS room, COALESCE(sum(b.acc),0) AS tacc FROM b
    )
    SELECT b.good_key,
           b.base + CASE WHEN t.tacc <= t.room THEN b.acc WHEN t.tacc > 0 THEN t.room * b.acc / t.tacc ELSE 0 END,
           b.base, b.acc,
           CASE WHEN t.tacc <= t.room THEN b.acc WHEN t.tacc > 0 THEN t.room * b.acc / t.tacc ELSE 0 END
      FROM b CROSS JOIN t
$$;

-- storage_capped: pool goods realize against the effective cap. One dispatch (the
-- StorageOverflow notice carrying pool='storehouse') when the production spell BEGINS,
-- never per good and never per tick; the StorageOverflow event is still written for
-- every lost quantity (the audit / the ledger's overflow).
CREATE OR REPLACE FUNCTION storage_capped(p_sid UUID,p_good TEXT,p_available DOUBLE PRECISION,p_cap DOUBLE PRECISION,p_reason TEXT)
RETURNS DOUBLE PRECISION LANGUAGE plpgsql VOLATILE AS $$
DECLARE lost DOUBLE PRECISION; wid UUID; owner UUID; pname TEXT; pid UUID;
        tick INT; body JSONB; eid BIGINT; nid UUID;
        pooled BOOLEAN := storehouse_stored_good(p_good); cap DOUBLE PRECISION := p_cap;
        e_base DOUBLE PRECISION; e_adm DOUBLE PRECISION; e_acc DOUBLE PRECISION; pool_amt DOUBLE PRECISION;
        announce BOOLEAN;
BEGIN
    IF pooled AND p_reason IN ('production','refining') THEN
        SELECT e.base + e.admitted, e.base, e.admitted, e.accrual INTO cap, e_base, e_adm, e_acc
          FROM storehouse_effective(p_sid, p_good, p_available) e WHERE e.good_key = p_good;
        IF NOT FOUND THEN cap := p_cap; ELSE
            cap := LEAST(cap, p_cap);
            PERFORM set_config('megaron.storehouse_clipped', p_sid::text||':'||p_good, true);
            IF p_good = 'silver' THEN PERFORM set_config('megaron.silver_admitted', GREATEST(0, LEAST(e_adm, cap - e_base))::text, true); END IF;
        END IF;
    END IF;
    lost:=GREATEST(0,p_available-cap);
    IF lost>0 THEN
        PERFORM set_config('megaron.ledger_overflow', (COALESCE(NULLIF(current_setting('megaron.ledger_overflow',true),''),'0')::float8 + lost)::text, true);
        SELECT s.world_id,s.owner_id,s.name,s.province_id,w.current_tick
        INTO wid,owner,pname,pid,tick FROM settlements s JOIN worlds w ON w.id=s.world_id WHERE s.id=p_sid;
        IF wid IS NULL THEN RAISE EXCEPTION 'storage outcome has no settlement %',p_sid; END IF;
        body:=jsonb_build_object('settlement_id',p_sid,'province_id',pid,'name',pname,
            'good_key',p_good,'lost_quantity',lost,'cap',cap,'reason',p_reason,
            'world_tick',tick,'player_id',owner);
        IF pooled THEN
            SELECT COALESCE(sum(e.amount),0) INTO pool_amt FROM storehouse_effective(p_sid, p_good, p_available) e;
            body := body || jsonb_build_object('pool','storehouse','pool_capacity',storehouse_cap(p_sid),
                'pool_amount',pool_amt,'pool_level',settlement_storehouse_level(p_sid));
        END IF;
        INSERT INTO events(stream_id,stream_type,event_type,payload,world_id)
        VALUES(p_sid,'province','StorageOverflow',body,wid) RETURNING id INTO eid;
        IF pooled AND p_reason = 'production' THEN
            -- One dispatch per production spell for the WHOLE store: none if any pooled good
            -- lost production in the last quiet window (an ongoing spell), none if one was
            -- already sent this tick (another good of the same realization).
            announce := owner IS NOT NULL
              AND NOT EXISTS(SELECT 1 FROM events e WHERE e.stream_id=p_sid AND e.id<>eid
                  AND e.event_type='StorageOverflow' AND e.payload->>'pool'='storehouse'
                  AND e.payload->>'reason'='production' AND (e.payload->>'world_tick')::int > tick-storage_overflow_quiet_ticks())
              AND NOT EXISTS(SELECT 1 FROM notifications n WHERE n.player_id=owner AND n.kind='StorageOverflow'
                  AND n.body_json->>'settlement_id'=p_sid::text AND n.body_json->>'pool'='storehouse'
                  AND n.body_json->>'reason'='production' AND n.world_tick=tick);
        ELSE
            announce := owner IS NOT NULL AND (p_reason <> 'production' OR NOT EXISTS(
               SELECT 1 FROM events e WHERE e.stream_id=p_sid AND e.id<>eid
                  AND e.event_type='StorageOverflow' AND e.payload->>'good_key'=p_good
                  AND e.payload->>'reason'='production' AND (e.payload->>'world_tick')::int > tick-storage_overflow_quiet_ticks()));
        END IF;
        IF announce THEN
            INSERT INTO notifications(world_id,player_id,kind,level,body_json,world_tick)
            VALUES(wid,owner,'StorageOverflow',2,body,tick) RETURNING id INTO nid;
            INSERT INTO scheduled_events(world_id,event_type,payload,process_after,due_tick,priority)
            VALUES(wid,'StorageOverflowDelivery',jsonb_build_object('notification_id',nid,'event_id',eid),now(),tick,50);
        END IF;
    END IF;
    RETURN LEAST(cap,GREATEST(0,p_available));
END $$;

-- Writers that realize accrual with settled() but do not pass through storage_capped
-- (payments, debits that advance calc_tick) must not smuggle unclipped accrual past
-- the pool. The clip lands on the row being written; siblings are only READ, so no
-- extra row lock is taken (no new lock order). Name sorts before settlement_goods_ledger_upd.
CREATE FUNCTION settlement_goods_storehouse_clip() RETURNS TRIGGER LANGUAGE plpgsql AS $$
DECLARE cur INT := current_world_tick(); acc DOUBLE PRECISION; avail DOUBLE PRECISION;
        capi DOUBLE PRECISION; adm DOUBLE PRECISION; lost DOUBLE PRECISION;
BEGIN
    IF cur IS NULL OR OLD.rate <= 0 OR NEW.calc_tick <= OLD.calc_tick OR NOT storehouse_stored_good(NEW.good_key) THEN
        RETURN NEW;
    END IF;
    IF current_setting('megaron.storehouse_clipped', true) = NEW.settlement_id::text||':'||NEW.good_key THEN
        PERFORM set_config('megaron.storehouse_clipped', '', true);
        RETURN NEW;
    END IF;
    acc := OLD.rate * GREATEST(0, cur - OLD.calc_tick);
    IF acc <= 0 THEN RETURN NEW; END IF;
    avail := OLD.amount + acc;
    SELECT e.base + e.admitted, e.admitted INTO capi, adm FROM storehouse_effective(OLD.settlement_id, NEW.good_key, avail) e WHERE e.good_key = NEW.good_key;
    IF NOT FOUND THEN RETURN NEW; END IF;
    capi := LEAST(capi, NEW.cap);
    IF NEW.good_key = 'silver' THEN PERFORM set_config('megaron.silver_admitted', GREATEST(0, LEAST(adm, capi - (avail - acc)))::text, true); END IF;
    lost := avail - capi;
    IF lost > 1e-9 THEN
        PERFORM set_config('megaron.ledger_overflow', (COALESCE(NULLIF(current_setting('megaron.ledger_overflow',true),''),'0')::float8 + lost)::text, true);
        NEW.amount := GREATEST(0, NEW.amount - lost);
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER settlement_goods_clip_storehouse BEFORE UPDATE ON settlement_goods
    FOR EACH ROW EXECUTE FUNCTION settlement_goods_storehouse_clip();

-- Silver mined = what the store ADMITTED of the accrual, not rate x elapsed clipped at a
-- per-row cap (a pooled rejection made that inference false). Every silver realization that
-- advances calc_tick passes storage_capped or the clip trigger, both of which publish the
-- admitted amount in megaron.silver_admitted before this AFTER trigger runs.
CREATE OR REPLACE FUNCTION record_silver_mining_credit() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE credited DOUBLE PRECISION; admitted DOUBLE PRECISION; wid UUID;
BEGIN
    IF OLD.good_key <> 'silver' OR OLD.rate <= 0 OR NEW.calc_tick <= OLD.calc_tick THEN
        RETURN NEW;
    END IF;
    admitted := NULLIF(current_setting('megaron.silver_admitted', true), '')::float8;
    PERFORM set_config('megaron.silver_admitted', '', true);
    IF admitted IS NOT NULL THEN
        credited := GREATEST(0, admitted);
    ELSE
        -- A write that published nothing (no accrual passed the store's gate): the
        -- per-row inference of mig 179.
        credited := GREATEST(0,
            LEAST(OLD.cap, GREATEST(0, OLD.amount + OLD.rate * (NEW.calc_tick - OLD.calc_tick)))
            - LEAST(OLD.cap, GREATEST(0, OLD.amount)));
    END IF;
    IF credited > 0 THEN
        SELECT world_id INTO wid FROM settlements WHERE id = OLD.settlement_id;
        INSERT INTO silver_mining_totals(settlement_id, world_id, credited_silver) VALUES(OLD.settlement_id, wid, credited)
        ON CONFLICT (settlement_id) DO UPDATE SET
            credited_silver = silver_mining_totals.credited_silver + EXCLUDED.credited_silver;
    END IF;
    RETURN NEW;
END
$$;

-- credit_settlement_good: a pool good serializes on the city's pool lock FIRST (before
-- any row lock), realizes its own accrual against the effective cap, then admits at
-- most the room that is left in the whole store. Food, herd and the rest keep the
-- per-row path unchanged.
CREATE OR REPLACE FUNCTION credit_settlement_good(p_sid UUID,p_good TEXT,p_qty DOUBLE PRECISION,p_reason TEXT,p_default_cap DOUBLE PRECISION DEFAULT 1000000)
RETURNS DOUBLE PRECISION LANGUAGE plpgsql VOLATILE AS $$
DECLARE g settlement_goods%ROWTYPE; stock DOUBLE PRECISION; result DOUBLE PRECISION; credit_cap DOUBLE PRECISION;
BEGIN
    IF p_qty<0 THEN RAISE EXCEPTION 'negative storage credit'; END IF;
    IF storehouse_stored_good(p_good) THEN
        PERFORM pg_advisory_xact_lock(hashtextextended('storehouse:'||p_sid::text,0));
    END IF;
    INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick)
    VALUES(p_sid,p_good,0,0,p_default_cap,current_world_tick()) ON CONFLICT DO NOTHING;
    SELECT * INTO STRICT g FROM settlement_goods WHERE settlement_id=p_sid AND good_key=p_good FOR UPDATE;
    stock:=storage_capped(p_sid,p_good,settled(g.amount,g.rate,g.calc_tick),g.cap,'production');
    IF storehouse_stored_good(p_good) THEN
        UPDATE settlement_goods SET amount=stock,calc_tick=current_world_tick()
         WHERE settlement_id=p_sid AND good_key=p_good;
        PERFORM set_config('megaron.storehouse_clipped', '', true);
        PERFORM set_config('megaron.silver_admitted', '', true);
        SELECT stock + GREATEST(0, storehouse_cap(p_sid) - COALESCE(sum(e.amount),0)) INTO credit_cap
          FROM storehouse_effective(p_sid) e;
        credit_cap := LEAST(credit_cap, g.cap);
    ELSE
        credit_cap := g.cap;
    END IF;
    result:=storage_capped(p_sid,p_good,stock+p_qty,credit_cap,p_reason);
    PERFORM set_config('megaron.ledger_reason', p_reason, true);
    UPDATE settlement_goods SET amount=result,calc_tick=current_world_tick()
    WHERE settlement_id=p_sid AND good_key=p_good;
    PERFORM set_config('megaron.ledger_reason', '', true);
    RETURN result-stock;
END $$;
