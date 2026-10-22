BEGIN READ ONLY;
-- cities
SELECT w.id AS world_id,w.status AS world_status,w.current_tick,now() AS observed_at,s.id,s.name,s.state,s.population,p.username,p.wanax_name,
 COALESCE((SELECT max(level) FROM buildings b WHERE b.settlement_id=s.id AND b.building_type='temple'),0) AS temple_level,
 COALESCE((SELECT weight FROM settlement_labor sl WHERE sl.settlement_id=s.id AND sl.good_key='cult'),0) AS cult_weight,
 (SELECT count(*) FROM settlement_placement sp WHERE sp.settlement_id=s.id) AS placed_gubbar,
 (SELECT coalesce(jsonb_object_agg(good_key,n),'{}') FROM (SELECT good_key,count(*) n FROM settlement_placement sp WHERE sp.settlement_id=s.id GROUP BY good_key) x) AS placed_by_good
 FROM settlements s JOIN worlds w ON w.id=s.world_id LEFT JOIN players p ON p.id=s.owner_id
 WHERE s.state NOT IN ('sunk','collapsed','razed') ORDER BY w.id,s.name;
-- legacy_rows
SELECT good_key,count(*) AS rows,sum(weight) AS weight_sum FROM settlement_labor GROUP BY good_key ORDER BY good_key;
-- placement_constraints
SELECT conname,pg_get_constraintdef(oid) AS definition FROM pg_constraint WHERE conrelid='settlement_placement'::regclass ORDER BY conname;
-- cult_goods
SELECT key,status,weight FROM goods WHERE key='cult';
-- cult_production
SELECT good_key,building_type,rate_per_tick FROM production_rules WHERE good_key='cult';
COMMIT;
