-- Instrument real production credits when a silver tuple is re-anchored.
-- No historical events/balances are rewritten. The first V2 audit establishes
-- its own opening watermark; it never subtracts a V1 total.
CREATE TABLE silver_mining_totals (
    settlement_id UUID PRIMARY KEY,
    world_id UUID NOT NULL REFERENCES worlds(id) ON DELETE CASCADE,
    credited_silver DOUBLE PRECISION NOT NULL DEFAULT 0 CHECK (credited_silver >= 0)
);
CREATE INDEX silver_mining_totals_world ON silver_mining_totals(world_id);
CREATE TABLE silver_audit_epochs (
    world_id UUID PRIMARY KEY REFERENCES worlds(id) ON DELETE CASCADE,
    instrumentation_epoch UUID NOT NULL DEFAULT gen_random_uuid()
);

CREATE FUNCTION record_silver_mining_credit() RETURNS trigger LANGUAGE plpgsql AS $$
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
CREATE TRIGGER silver_mining_credit AFTER UPDATE OF amount, rate, cap, calc_tick ON settlement_goods
FOR EACH ROW EXECUTE FUNCTION record_silver_mining_credit();
