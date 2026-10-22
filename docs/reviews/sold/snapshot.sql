SELECT clock_timestamp() AS observed_at,current_setting('transaction_read_only') AS transaction_read_only,
 (SELECT version FROM schema_migrations) AS migration,
 (SELECT jsonb_agg(to_jsonb(w)) FROM (SELECT id,name,status,current_tick FROM worlds) w) AS worlds,
 (SELECT coalesce(jsonb_agg(to_jsonb(x)),'[]') FROM (
 SELECT u.id,u.world_id,w.status AS world_status,w.current_tick,u.owner_id,u.type,u.category,u.size,u.crew,u.status,
 u.settlement_id,u.support_settlement_id,u.unpaid_periods,
 loc.name AS location_name,pay.name AS payer_name,
 pay.id AS valid_payer_id,
 EXISTS(SELECT 1 FROM founder_phase fp WHERE fp.world_id=u.world_id AND fp.owner_id=u.owner_id AND fp.active) AS subsistence
 FROM units u JOIN worlds w ON w.id=u.world_id
 LEFT JOIN settlements loc ON loc.id=u.settlement_id
 LEFT JOIN settlements pay ON pay.id=u.support_settlement_id AND pay.owner_id=u.owner_id
 WHERE u.status<>'disbanded' ORDER BY w.status,u.type,u.id
 ) x) AS units
