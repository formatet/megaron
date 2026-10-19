-- Order 33 (hjorden v2): the herd breeds itself, there is no herder job.
-- Gubbar placed on livestock go back to the pool (their placement row goes, the
-- population is untouched), the plains livestock production rule goes, and any
-- livestock rate still on a settlement is zeroed after settling what it earned.
-- Idempotent: every statement is a no-op on a second run.
DELETE FROM settlement_placement WHERE good_key = 'livestock';
DELETE FROM production_rules WHERE good_key = 'livestock';
UPDATE settlement_goods
   SET amount = GREATEST(0, settled(amount, rate, calc_tick)),
       calc_tick = COALESCE(current_world_tick(), calc_tick),
       rate = 0
 WHERE good_key = 'livestock' AND rate <> 0 AND current_world_tick() IS NOT NULL;
UPDATE settlement_goods SET rate = 0 WHERE good_key = 'livestock' AND rate <> 0;
