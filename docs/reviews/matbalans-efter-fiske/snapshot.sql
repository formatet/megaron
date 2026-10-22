SELECT clock_timestamp() AS observed_at,current_setting('transaction_read_only') AS transaction_read_only,
 (SELECT row_to_json(m) FROM schema_migrations m) AS migration,
 (SELECT coalesce(jsonb_agg(to_jsonb(w)),'[]') FROM worlds w) AS worlds,
 (SELECT coalesce(jsonb_agg(to_jsonb(r)),'[]') FROM production_rules r) AS rules,
 (SELECT coalesce(jsonb_agg(to_jsonb(v)),'[]') FROM divine_labor_valuations v) AS valuations,
 (SELECT coalesce(jsonb_agg(to_jsonb(g)),'[]') FROM goods g) AS goods_catalog,
 (SELECT coalesce(jsonb_agg(to_jsonb(c)),'[]') FROM (
 SELECT s.*,p.map_q,p.map_r,
 (SELECT coalesce(jsonb_agg(to_jsonb(g)||jsonb_build_object('settled_stock',greatest(0,least(g.cap,settled(g.amount,g.rate,g.calc_tick))))),'[]') FROM settlement_goods g WHERE g.settlement_id=s.id) AS goods,
 (SELECT coalesce(jsonb_agg(to_jsonb(b)),'[]') FROM buildings b WHERE b.settlement_id=s.id) AS buildings,
 (SELECT coalesce(jsonb_agg(to_jsonb(j)),'[]') FROM settlement_placement j WHERE j.settlement_id=s.id) AS placements,
 (SELECT coalesce(jsonb_agg(to_jsonb(l)),'[]') FROM settlement_labor l WHERE l.settlement_id=s.id) AS labor,
 (SELECT coalesce(jsonb_agg(to_jsonb(t)),'[]') FROM (
 SELECT mt.*,(SELECT count(*) FROM (VALUES(1,0),(-1,0),(0,1),(0,-1),(1,-1),(-1,1)) d(dq,dr)
 JOIN map_tiles n ON n.world_id=mt.world_id AND n.q=mt.q+d.dq AND n.r=mt.r+d.dr WHERE n.terrain NOT IN ('sea','coastal_sea','deep_sea')) AS land_neighbors
 FROM map_tiles mt WHERE mt.world_id=s.world_id AND greatest(abs(mt.q-p.map_q),abs(mt.r-p.map_r),abs(mt.q-p.map_q+mt.r-p.map_r)) BETWEEN 1 AND 2 ORDER BY mt.q,mt.r) t) AS catchment
 FROM settlements s JOIN provinces p ON p.id=s.province_id WHERE s.state NOT IN ('sunk','collapsed','razed') ORDER BY s.world_id,s.name) c) AS cities,
 (SELECT coalesce(jsonb_agg(to_jsonb(u)),'[]') FROM units u WHERE u.status<>'disbanded') AS units,
 (SELECT coalesce(jsonb_agg(to_jsonb(r)),'[]') FROM transports r WHERE r.ship_unit_id IS NOT NULL) AS transports,
 (SELECT coalesce(jsonb_agg(to_jsonb(f)),'[]') FROM founder_phase f WHERE f.active) AS founders,
 (SELECT coalesce(jsonb_agg(to_jsonb(e)),'[]') FROM (
 SELECT e.id,e.stream_id,e.world_id,e.world_tick,e.event_type,e.payload FROM events e
 WHERE e.event_type IN ('UpkeepSettled','FoodShortfall','SilverAuditV2','HerdBirths','HerdLost','KharisDailyTick','KharisMaintenance')
 AND e.world_tick >= (SELECT w.current_tick-32 FROM worlds w WHERE w.id=e.world_id)
 ORDER BY e.id) e) AS recent_events
