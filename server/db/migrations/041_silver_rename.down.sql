ALTER TABLE settlements
  RENAME COLUMN silver_amount  TO gold_amount;
ALTER TABLE settlements
  RENAME COLUMN silver_rate    TO gold_rate;
ALTER TABLE settlements
  RENAME COLUMN silver_cap     TO gold_cap;
ALTER TABLE settlements
  RENAME COLUMN silver_calc_at TO gold_calc_at;

ALTER TABLE kingdoms
  RENAME COLUMN silver_amount  TO gold_amount;
ALTER TABLE kingdoms
  RENAME COLUMN silver_rate    TO gold_rate;
ALTER TABLE kingdoms
  RENAME COLUMN silver_cap     TO gold_cap;
ALTER TABLE kingdoms
  RENAME COLUMN silver_calc_at TO gold_calc_at;

UPDATE messengers
   SET trade_offer = trade_offer - 'offer_silver'
                  || jsonb_build_object('offer_gold', (trade_offer->>'offer_silver')::float)
 WHERE trade_offer ? 'offer_silver';
