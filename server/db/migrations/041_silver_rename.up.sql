-- Sprint A: rename gold_* columns → silver_* on settlements and kingdoms.
-- The silver shekel has always been the currency; gold is reserved for the future luxury good.

ALTER TABLE settlements
  RENAME COLUMN gold_amount  TO silver_amount;
ALTER TABLE settlements
  RENAME COLUMN gold_rate    TO silver_rate;
ALTER TABLE settlements
  RENAME COLUMN gold_cap     TO silver_cap;
ALTER TABLE settlements
  RENAME COLUMN gold_calc_at TO silver_calc_at;

ALTER TABLE kingdoms
  RENAME COLUMN gold_amount  TO silver_amount;
ALTER TABLE kingdoms
  RENAME COLUMN gold_rate    TO silver_rate;
ALTER TABLE kingdoms
  RENAME COLUMN gold_cap     TO silver_cap;
ALTER TABLE kingdoms
  RENAME COLUMN gold_calc_at TO silver_calc_at;

-- Rename offer_gold → offer_silver in messenger trade-offer JSONB payloads.
UPDATE messengers
   SET trade_offer = trade_offer - 'offer_gold'
                  || jsonb_build_object('offer_silver', (trade_offer->>'offer_gold')::float)
 WHERE trade_offer ? 'offer_gold';
