-- Skip unrelated goods before entering silver instrumentation. Keep the
-- production-credit function, counters, epochs and recorded outcomes intact.
DROP TRIGGER silver_mining_credit ON settlement_goods;
CREATE TRIGGER silver_mining_credit
AFTER UPDATE OF amount, rate, cap, calc_tick ON settlement_goods
FOR EACH ROW WHEN (NEW.good_key = 'silver')
EXECUTE FUNCTION record_silver_mining_credit();
