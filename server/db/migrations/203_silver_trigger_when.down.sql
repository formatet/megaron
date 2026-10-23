-- Restore migration 179's trigger dispatch, without resetting instrumentation.
DROP TRIGGER silver_mining_credit ON settlement_goods;
CREATE TRIGGER silver_mining_credit
AFTER UPDATE OF amount, rate, cap, calc_tick ON settlement_goods
FOR EACH ROW EXECUTE FUNCTION record_silver_mining_credit();
