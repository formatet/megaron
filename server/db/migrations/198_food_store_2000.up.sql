-- Food store ceiling 2 000 per good with no Granary; Granary 1/2/3 gives
-- 4 000 / 10 000 / 20 000 (Timothy 2026-10-21). Livestock and all other goods keep
-- their technical ceiling. The base is owned by province.FoodStorageBase.
--
-- Stocks already above the new ceiling are KEPT (Timothy 2026-10-21, as with the
-- wall: nobody loses what they have). Such a row's cap starts at its stock and
-- only ratchets DOWN as the city eats, never back up, until it meets the ceiling:
-- the old surplus can be eaten, never refilled.
CREATE FUNCTION food_store_base() RETURNS INT LANGUAGE sql IMMUTABLE AS $$ SELECT 2000 $$;

CREATE FUNCTION food_store_cap(p_sid UUID) RETURNS DOUBLE PRECISION LANGUAGE sql STABLE AS $$
    SELECT (food_store_base() * silo_food_mult(settlement_silo_level(p_sid)))::double precision
$$;

-- A new food row: the ceiling, or its own opening stock if that is larger.
CREATE OR REPLACE FUNCTION settlement_goods_silo_cap() RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    IF silo_stored_good(NEW.good_key) THEN
        NEW.cap := GREATEST(food_store_cap(NEW.settlement_id), NEW.amount);
    END IF;
    RETURN NEW;
END $$;

-- Granary built, levelled or lost: the ceiling follows, but never below what is
-- in the store right now (a kept surplus then ratchets down as it is eaten).
CREATE OR REPLACE FUNCTION buildings_silo_cap() RETURNS TRIGGER LANGUAGE plpgsql AS $$
DECLARE sid UUID; btype TEXT;
BEGIN
    IF TG_OP = 'DELETE' THEN sid := OLD.settlement_id; btype := OLD.building_type;
    ELSE sid := NEW.settlement_id; btype := NEW.building_type; END IF;
    IF btype <> 'silo' THEN RETURN NULL; END IF;
    UPDATE settlement_goods
       SET cap = GREATEST(food_store_cap(sid), LEAST(cap, settled(amount, rate, calc_tick)))
     WHERE settlement_id = sid AND silo_stored_good(good_key)
       AND cap <> GREATEST(food_store_cap(sid), LEAST(cap, settled(amount, rate, calc_tick)));
    RETURN NULL;
END $$;

-- The ratchet: a kept surplus (cap above the ceiling) shrinks with the stock.
CREATE FUNCTION settlement_goods_food_ratchet() RETURNS TRIGGER LANGUAGE plpgsql AS $$
DECLARE ceiling DOUBLE PRECISION;
BEGIN
    IF silo_stored_good(NEW.good_key) AND NEW.cap > food_store_base() THEN
        ceiling := food_store_cap(NEW.settlement_id);
        IF NEW.cap > ceiling THEN
            NEW.cap := GREATEST(ceiling, LEAST(NEW.cap, settled(NEW.amount, NEW.rate, NEW.calc_tick)));
        END IF;
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER settlement_goods_food_ratchet BEFORE UPDATE ON settlement_goods
    FOR EACH ROW EXECUTE FUNCTION settlement_goods_food_ratchet();

-- Overflow notice once per spell of overflowing production, not once per tick.
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
        -- ticks). The event above is still written each time — it is the outcome.
        IF owner IS NOT NULL AND (p_reason <> 'production' OR NOT EXISTS(
               SELECT 1 FROM events e WHERE e.stream_id=p_sid AND e.id<>eid
                  AND e.event_type='StorageOverflow' AND e.payload->>'good_key'=p_good
                  AND e.payload->>'reason'='production' AND (e.payload->>'world_tick')::int >= tick-1)) THEN
            INSERT INTO notifications(world_id,player_id,kind,level,body_json,world_tick)
            VALUES(wid,owner,'StorageOverflow',2,body,tick) RETURNING id INTO nid;
            INSERT INTO scheduled_events(world_id,event_type,payload,process_after,due_tick,priority)
            VALUES(wid,'StorageOverflowDelivery',jsonb_build_object('notification_id',nid,'event_id',eid),now(),tick,50);
        END IF;
    END IF;
    RETURN LEAST(p_cap,GREATEST(0,p_available));
END $$;

-- Existing rows: ceiling, or the stock held today if larger. No stock changes.
UPDATE settlement_goods
   SET cap = GREATEST(food_store_cap(settlement_id), LEAST(cap, settled(amount, rate, calc_tick)))
 WHERE silo_stored_good(good_key);
