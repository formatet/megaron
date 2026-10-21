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
        IF owner IS NOT NULL THEN
            INSERT INTO notifications(world_id,player_id,kind,level,body_json,world_tick)
            VALUES(wid,owner,'StorageOverflow',2,body,tick) RETURNING id INTO nid;
            INSERT INTO scheduled_events(world_id,event_type,payload,process_after,due_tick,priority)
            VALUES(wid,'StorageOverflowDelivery',jsonb_build_object('notification_id',nid,'event_id',eid),now(),tick,50);
        END IF;
    END IF;
    RETURN LEAST(p_cap,GREATEST(0,p_available));
END $$;

DROP TRIGGER settlement_goods_food_ratchet ON settlement_goods;
DROP FUNCTION settlement_goods_food_ratchet();

CREATE OR REPLACE FUNCTION settlement_goods_silo_cap() RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    IF silo_stored_good(NEW.good_key) THEN
        NEW.cap := GREATEST(NEW.cap, 1000000 * silo_food_mult(settlement_silo_level(NEW.settlement_id)));
    END IF;
    RETURN NEW;
END $$;

CREATE OR REPLACE FUNCTION buildings_silo_cap() RETURNS TRIGGER LANGUAGE plpgsql AS $$
DECLARE sid UUID; btype TEXT;
BEGIN
    IF TG_OP = 'DELETE' THEN sid := OLD.settlement_id; btype := OLD.building_type;
    ELSE sid := NEW.settlement_id; btype := NEW.building_type; END IF;
    IF btype <> 'silo' THEN RETURN NULL; END IF;
    UPDATE settlement_goods SET cap = 1000000 * silo_food_mult(settlement_silo_level(sid))
     WHERE settlement_id = sid AND silo_stored_good(good_key)
       AND cap <> 1000000 * silo_food_mult(settlement_silo_level(sid));
    RETURN NULL;
END $$;

UPDATE settlement_goods SET cap = 1000000 * silo_food_mult(settlement_silo_level(settlement_id))
 WHERE silo_stored_good(good_key);

DROP FUNCTION food_store_cap(UUID);
DROP FUNCTION food_store_base();
