-- K4 API tick-contract: expose a march's timing in world ticks, not just wall-clock.
--
-- The tick substrate (mig 067) made worlds.current_tick the source of truth for
-- timing; units already cache departs_at/arrives_at (wall-clock) on the row so the
-- map can interpolate a marcher's position. These two columns are the tick-native
-- mirror of that pair — written in the SAME UPDATE at every course-setting site
-- (march dispatch, recall/redirect, explore-return, rout-return) and NULLed
-- wherever arrives_at is NULLed on arrival. From them the API derives arrival_tick,
-- duration_ticks (arrive_tick − depart_tick), and a derived arrives_at_utc
-- (tick.EtaAt). Nullable, no default: only a marching unit carries them.
ALTER TABLE units ADD COLUMN depart_tick INT;
ALTER TABLE units ADD COLUMN arrive_tick INT;
