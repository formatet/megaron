SELECT now() AS observed_at, current_setting('transaction_read_only') AS transaction_read_only,
 (SELECT version FROM schema_migrations) AS migration,
 (SELECT jsonb_agg(to_jsonb(w)) FROM (SELECT id,name,status,current_tick FROM worlds WHERE id IN (SELECT DISTINCT world_id FROM settlements WHERE state NOT IN ('sunk','collapsed','razed'))) w) AS worlds,
 (SELECT jsonb_agg(to_jsonb(pr)) FROM (SELECT * FROM production_rules WHERE good_key IN ('grain','fish')) pr) AS food_rules,
 (SELECT jsonb_agg(to_jsonb(t)) FROM (SELECT world_id,terrain,count(*) AS count FROM map_tiles WHERE world_id IN (SELECT DISTINCT world_id FROM settlements WHERE state NOT IN ('sunk','collapsed','razed')) GROUP BY world_id,terrain) t) AS terrain_counts,
 (SELECT jsonb_agg(to_jsonb(c)) FROM (
  SELECT s.id,s.name,s.world_id,s.state,s.population,s.harvest_blessing_until_tick,p.map_q AS q,p.map_r AS r,
   (SELECT coalesce(jsonb_agg(to_jsonb(b)),'[]') FROM (SELECT building_type,level,hex_q,hex_r FROM buildings WHERE settlement_id=s.id) b) AS buildings,
   (SELECT coalesce(jsonb_agg(to_jsonb(j)),'[]') FROM (SELECT gubbe_ordinal,target_kind,good_key,building_type,hex_q,hex_r FROM settlement_placement WHERE settlement_id=s.id) j) AS placements,
   (SELECT coalesce(jsonb_agg(to_jsonb(g)),'[]') FROM (SELECT good_key,amount,rate,cap,calc_tick FROM settlement_goods WHERE settlement_id=s.id AND good_key IN ('grain','fish','oil','wine','livestock')) g) AS food_goods,
   (SELECT coalesce(jsonb_agg(to_jsonb(t)),'[]') FROM (
    SELECT mt.q,mt.r,mt.terrain,mt.coastal,mt.copper_deposit,mt.tin_deposit,mt.silver_deposit,mt.cedar_deposit,mt.incense_deposit,
     (SELECT coalesce(jsonb_agg(to_jsonb(n)),'[]') FROM (
      SELECT nx.q,nx.r,nx.terrain FROM (VALUES(1,0),(-1,0),(0,1),(0,-1),(1,-1),(-1,1)) d(dq,dr)
      JOIN map_tiles nx ON nx.world_id=mt.world_id AND nx.q=mt.q+d.dq AND nx.r=mt.r+d.dr
      ORDER BY nx.q,nx.r
     ) n) AS neighbours
    FROM map_tiles mt WHERE mt.world_id=s.world_id
    AND greatest(abs(mt.q-p.map_q),abs(mt.r-p.map_r),abs(mt.q-p.map_q+mt.r-p.map_r)) BETWEEN 1 AND 2 ORDER BY mt.q,mt.r
   ) t) AS catchment
  FROM settlements s JOIN provinces p ON p.id=s.province_id WHERE s.state NOT IN ('sunk','collapsed','razed') ORDER BY s.world_id,s.name
 ) c) AS cities
