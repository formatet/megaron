BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT json_build_object(
 'read_only', current_setting('transaction_read_only'),
 'captured_at', current_timestamp,
 'migration', (SELECT max(version) FROM schema_migrations),
 'worlds', (SELECT json_agg(x ORDER BY x.id) FROM (SELECT id,map_seed,map_width,map_height,current_tick FROM worlds) x),
 'rules', (SELECT json_agg(x ORDER BY x.id) FROM production_rules x),
 'tiles', (SELECT json_agg(x ORDER BY x.world_id,x.q,x.r) FROM (SELECT world_id,q,r,terrain::text,coastal FROM map_tiles) x),
 'settlements', (SELECT json_agg(x ORDER BY x.id) FROM (SELECT s.id,s.name,s.state,s.population,p.world_id,p.map_q AS q,p.map_r AS r FROM settlements s JOIN provinces p ON p.id=s.province_id) x)
);
COMMIT;
