-- Forward-only data collapse: which records were 'active' before the archive
-- cannot be reconstructed. Reactivate every archived record to the broadest
-- reversible approximation (the unique index is on worlds, not these rows).
UPDATE player_world_records SET status = 'active' WHERE status = 'archived';
