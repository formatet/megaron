-- Rows whose world is gone would block the key; drop them before restoring it.
DELETE FROM player_reports pr WHERE NOT EXISTS (SELECT 1 FROM worlds w WHERE w.id = pr.world_id);
ALTER TABLE player_reports
    ADD CONSTRAINT player_reports_world_id_fkey FOREIGN KEY (world_id) REFERENCES worlds(id) ON DELETE CASCADE;
