ALTER TABLE sea_storm_progress ADD COLUMN last_milli bigint;
ALTER TABLE sea_storm_progress ADD COLUMN last_storm_id uuid;
CREATE TABLE timed_route_progress (
 world_id uuid PRIMARY KEY REFERENCES worlds(id) ON DELETE CASCADE,
 last_milli bigint NOT NULL
);
