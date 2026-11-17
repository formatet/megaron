CREATE TABLE departure_prayers (
 settlement_id UUID NOT NULL REFERENCES settlements(id) ON DELETE CASCADE,
 owner_id UUID NOT NULL REFERENCES players(id) ON DELETE CASCADE,
 effect TEXT NOT NULL CHECK(effect IN ('next_land_departure','next_ship_departure')),
 PRIMARY KEY(settlement_id,effect)
);
