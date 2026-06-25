-- Single-world enforcement: collapse stale player_world_records so a Wanax is
-- only 'active' in the one live world. Accumulated reseed history (records in
-- archived worlds) goes 'archived'. 'dispossessed' records are left untouched.
-- Going forward, world creation cascades this archive (handlers/world.go).
UPDATE player_world_records
   SET status = 'archived'
 WHERE status = 'active'
   AND world_id NOT IN (SELECT id FROM worlds WHERE status = 'active');
