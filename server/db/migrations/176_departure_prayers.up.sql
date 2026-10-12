CREATE TABLE departure_prayers (
 settlement_id UUID NOT NULL REFERENCES settlements(id) ON DELETE CASCADE,
 owner_id UUID NOT NULL REFERENCES players(id) ON DELETE CASCADE,
 effect TEXT NOT NULL CHECK(effect IN ('next_land_departure','next_ship_departure')),
 PRIMARY KEY(settlement_id,effect)
);
ALTER TABLE units ADD COLUMN stillness_depart_tick INT;
ALTER TABLE units ADD COLUMN stillness_arrive_tick INT;
ALTER TABLE units ADD COLUMN stillness_owner_id UUID REFERENCES players(id) ON DELETE SET NULL;
