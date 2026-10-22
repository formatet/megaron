BEGIN READ ONLY;
SELECT s.id,s.name,s.population,w.id AS world_id,w.status AS world_status,now() AS observed_at,
 coalesce((SELECT max(level) FROM buildings b WHERE b.settlement_id=s.id AND b.building_type='temple'),0) AS level,
 coalesce((SELECT weight FROM settlement_labor l WHERE l.settlement_id=s.id AND l.good_key='cult'),0) AS weight,
 coalesce((SELECT jsonb_agg(jsonb_build_object('ordinal',gubbe_ordinal,'good',good_key,'kind',target_kind,'building',building_type,'q',hex_q,'r',hex_r) ORDER BY gubbe_ordinal) FROM settlement_placement sp WHERE sp.settlement_id=s.id),'[]') AS placements
 FROM settlements s JOIN worlds w ON w.id=s.world_id WHERE s.state NOT IN ('sunk','collapsed','razed') ORDER BY s.id;
COMMIT;
