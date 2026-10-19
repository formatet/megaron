-- Actual civilian meal, claimed and written with FoodTick's existing TX.
ALTER TABLE settlements ADD COLUMN food_consumed JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE settlements ADD COLUMN food_consumed_tick INT;
-- Preserve the commodity identity of provisions through loading/eating/return.
ALTER TABLE units ADD COLUMN provision_goods JSONB NOT NULL DEFAULT '{}'::jsonb;
UPDATE units SET provision_goods=jsonb_build_object('grain',provisions) WHERE provisions>0;
