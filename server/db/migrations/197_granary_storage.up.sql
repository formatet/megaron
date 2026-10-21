-- Granary (building_type 'silo'): multiplies the storage ceiling of every food
-- except livestock (the herd has its pens) — x2 / x5 / x10 at level 1/2/3
-- (Timothy 2026-10-21, megaron_plan_budvagar_och_murtak B0). The numbers are owned
-- by province.SiloFoodStorageMult; TestSiloStorageSQLMirror pins this to it.
-- The ceiling is the per-row settlement_goods.cap (base 1 000 000), so the
-- invariant is kept at every moment a row can come into being or a Granary change:
--   * a BEFORE INSERT trigger on settlement_goods raises a new food row's cap;
--   * an AFTER trigger on buildings re-caps the existing food rows when the
--     Granary is built, levelled or lost.
CREATE FUNCTION silo_food_mult(p_level INT) RETURNS INT LANGUAGE sql IMMUTABLE AS $$
    SELECT CASE WHEN p_level >= 3 THEN 10 WHEN p_level = 2 THEN 5 WHEN p_level = 1 THEN 2 ELSE 1 END
$$;

-- Food goods that live in the store (economy.FoodGoods); livestock is excluded.
CREATE FUNCTION silo_stored_good(p_good TEXT) RETURNS BOOLEAN LANGUAGE sql IMMUTABLE AS $$
    SELECT p_good IN ('grain','fish','oil','wine')
$$;

CREATE FUNCTION settlement_silo_level(p_sid UUID) RETURNS INT LANGUAGE sql STABLE AS $$
    SELECT COALESCE(MAX(level),0)::int FROM buildings WHERE settlement_id=p_sid AND building_type='silo'
$$;

CREATE FUNCTION settlement_goods_silo_cap() RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    IF silo_stored_good(NEW.good_key) THEN
        NEW.cap := GREATEST(NEW.cap, 1000000 * silo_food_mult(settlement_silo_level(NEW.settlement_id)));
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER settlement_goods_silo_cap BEFORE INSERT ON settlement_goods
    FOR EACH ROW EXECUTE FUNCTION settlement_goods_silo_cap();

CREATE FUNCTION buildings_silo_cap() RETURNS TRIGGER LANGUAGE plpgsql AS $$
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
CREATE TRIGGER buildings_silo_cap AFTER INSERT OR UPDATE OF level OR DELETE ON buildings
    FOR EACH ROW EXECUTE FUNCTION buildings_silo_cap();
