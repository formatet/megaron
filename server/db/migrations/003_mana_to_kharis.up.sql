-- Rename mana resource → kharis throughout provinces table
ALTER TABLE provinces
  RENAME COLUMN mana_amount  TO kharis_amount;
ALTER TABLE provinces
  RENAME COLUMN mana_rate    TO kharis_rate;
ALTER TABLE provinces
  RENAME COLUMN mana_cap     TO kharis_cap;
ALTER TABLE provinces
  RENAME COLUMN mana_calc_at TO kharis_calc_at;
