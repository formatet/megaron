SELECT clock_timestamp() AS observed_at,current_setting('transaction_read_only') AS transaction_read_only,
 (SELECT row_to_json(m) FROM schema_migrations m) AS migration,
 (SELECT id FROM worlds WHERE id::text LIKE '2178e795%') AS world_id,
 (SELECT current_tick FROM worlds WHERE id::text LIKE '2178e795%') AS tick,
 (SELECT coalesce(jsonb_agg(jsonb_build_object('player_id',r.player_id,'username',p.username,'kharis_amount',r.kharis_amount,'kharis_rate',r.kharis_rate,'kharis_calc_tick',r.kharis_calc_tick)),'[]') FROM player_world_records r JOIN players p ON p.id=r.player_id WHERE r.world_id::text LIKE '2178e795%') AS wanaxes,
 (SELECT coalesce(jsonb_object_agg(good_key,divine_value),'{}') FROM divine_labor_valuations WHERE world_id::text LIKE '2178e795%') AS values,
 (SELECT coalesce(jsonb_object_agg(key,labor_value),'{}') FROM goods WHERE status='active' AND labor_value IS NOT NULL) AS base_values,
 (SELECT coalesce(jsonb_agg(to_jsonb(c)),'[]') FROM (
 SELECT s.id,s.name,s.owner_id,s.population,s.state,
 (SELECT coalesce(jsonb_object_agg(g.good_key,greatest(0,settled(g.amount,g.rate,g.calc_tick))),'{}') FROM settlement_goods g WHERE g.settlement_id=s.id AND g.good_key IN ('grain','fish','timber','silver')) AS stock,
 (SELECT coalesce(jsonb_agg(jsonb_build_object('id',b.id,'god_id',b.god_id,'level',b.level,'staffed',least(2*b.level,(SELECT count(*) FROM settlement_placement p WHERE p.settlement_id=s.id AND p.building_id=b.id AND p.good_key='cult'))) ORDER BY b.god_id),'[]') FROM buildings b WHERE b.settlement_id=s.id AND b.building_type='temple') AS temples
 FROM settlements s WHERE s.world_id::text LIKE '2178e795%' AND s.state NOT IN ('sunk','collapsed','razed') ORDER BY s.name,s.id) c) AS cities
