ALTER TABLE units ADD COLUMN stillness_depart_tick INT;
ALTER TABLE units ADD COLUMN stillness_arrive_tick INT;
ALTER TABLE units ADD COLUMN stillness_owner_id UUID REFERENCES players(id) ON DELETE SET NULL;
