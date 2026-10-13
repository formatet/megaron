BEGIN READ ONLY;
WITH occupied AS (
 SELECT map_q AS q,map_r AS r FROM provinces WHERE world_id='f7c90afa-4653-46a6-b14c-473a268efa3e'
 UNION ALL SELECT u.q,u.r FROM units u JOIN founder_phase f ON f.host_unit_id=u.id AND f.active
 WHERE u.world_id='f7c90afa-4653-46a6-b14c-473a268efa3e' AND u.q IS NOT NULL AND u.r IS NOT NULL
), eligible AS (
 SELECT mt.* FROM map_tiles mt
 WHERE mt.world_id='f7c90afa-4653-46a6-b14c-473a268efa3e'
 AND mt.terrain NOT IN ('coastal_sea','deep_sea','river','river_ford','mountain_limestone','mountain_red','semi_desert')
 AND (mt.landmass_id IS NULL OR mt.landmass_id IN (
   SELECT vt.landmass_id FROM map_tiles vt WHERE vt.world_id=mt.world_id
   AND vt.terrain IN (SELECT terrain_type FROM production_rules WHERE good_key='timber' AND building_type IS NULL AND terrain_type IS NOT NULL)))
 AND NOT EXISTS (SELECT 1 FROM occupied o WHERE (ABS(mt.q-o.q)+ABS(mt.r-o.r)+ABS(mt.q+mt.r-o.q-o.r))/2 <= 4)
), matched AS (
 SELECT e.q,e.r,
   COALESCE(bool_or(n.terrain IN ('coastal_sea','deep_sea')),false) AS coast,
   COALESCE(bool_or(n.silver_deposit),false) AS silver,
   COALESCE(bool_or(n.terrain IN (SELECT terrain_type FROM production_rules WHERE good_key='timber' AND building_type IS NULL)),false) AS forest,
   COALESCE(bool_or(n.terrain IN (SELECT terrain_type FROM production_rules WHERE good_key='grain' AND building_type IS NULL)),false) AS grain,
   COALESCE(bool_or(n.terrain IN (SELECT terrain_type FROM production_rules WHERE good_key='fish' AND building_type IS NULL)),false) AS fish
 FROM eligible e LEFT JOIN map_tiles n ON n.world_id=e.world_id
 AND (ABS(e.q-n.q)+ABS(e.r-n.r)+ABS(e.q+e.r-n.q-n.r))/2 BETWEEN 1 AND 2
 GROUP BY e.q,e.r
)
SELECT json_build_object('candidates',count(*),'forest',count(*) FILTER (WHERE forest),
 'coast',count(*) FILTER (WHERE coast),'silver',count(*) FILTER (WHERE silver),
 'grain',count(*) FILTER (WHERE grain),'fish',count(*) FILTER (WHERE fish),
 'grain_or_fish',count(*) FILTER (WHERE grain OR fish),
 'any_early_good',count(*) FILTER (WHERE grain OR fish OR forest OR silver)) FROM matched;
COMMIT;
