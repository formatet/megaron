-- Nothing sets the units.stillness_* columns since the departure prayers were removed (256);
-- the storm and speed-up readers are gone. Live count at removal: depart/arrive ticks 0 rows set;
-- stillness_owner_id was only ever stamped with the ship owner by transport dispatch (5 of 6 units).
ALTER TABLE units DROP COLUMN stillness_depart_tick, DROP COLUMN stillness_arrive_tick, DROP COLUMN stillness_owner_id;
