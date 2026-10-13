BEGIN READ ONLY;
SELECT json_build_object('kind','migration','data',row_to_json(s)) FROM schema_migrations s;
SELECT json_build_object('kind','timber_terrains','data',json_agg(terrain_type ORDER BY terrain_type)) FROM production_rules WHERE good_key='timber' AND building_type IS NULL AND terrain_type IS NOT NULL;
SELECT json_build_object('kind','units','data',row_to_json(s)) FROM (
 SELECT u.world_id,u.type,u.category,u.status,u.size,
 u.settlement_id IS NOT NULL AND u.settlement_id=u.support_settlement_id AS own_payer,
 EXISTS(SELECT 1 FROM founder_phase fp WHERE fp.world_id=u.world_id AND fp.owner_id=u.owner_id AND fp.active) AS active_founder,
 EXISTS(SELECT 1 FROM settlements s WHERE s.id=u.support_settlement_id AND s.owner_id=u.owner_id) AS valid_payer,
 u.unpaid_periods,COUNT(*) AS units
 FROM units u WHERE u.status IN ('garrison','marching','positioned','embarked','repairing','freighting')
 GROUP BY u.world_id,u.type,u.category,u.status,u.size,u.settlement_id IS NOT NULL AND u.settlement_id=u.support_settlement_id,
 EXISTS(SELECT 1 FROM founder_phase fp WHERE fp.world_id=u.world_id AND fp.owner_id=u.owner_id AND fp.active),
 EXISTS(SELECT 1 FROM settlements s WHERE s.id=u.support_settlement_id AND s.owner_id=u.owner_id),u.unpaid_periods
 ORDER BY u.world_id,u.type,u.status,u.size) s;
SELECT json_build_object('kind','world','data',row_to_json(s)) FROM (SELECT id,map_width,map_height,map_seed,current_tick,status FROM worlds) s;
SELECT json_build_object('kind','wanaxes','data',row_to_json(s)) FROM (SELECT world_id,count(*) FROM player_world_records GROUP BY world_id) s;
SELECT json_build_object('kind','founders','data',row_to_json(s)) FROM (SELECT world_id,count(*) AS hosts,sum(silver_rate) AS silver_rate,sum(silver_amount) AS silver_amount FROM founder_phase WHERE active GROUP BY world_id) s;
SELECT json_build_object('kind','silver_tiles','data',row_to_json(s)) FROM (SELECT world_id,terrain,count(*) FROM map_tiles WHERE silver_deposit GROUP BY world_id,terrain) s;
COMMIT;
