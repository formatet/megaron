UPDATE map_tiles SET terrain='coastal_sea',coastal=false,river_edges=NULL,river_upstream_edges=NULL,river_downstream_edges=NULL,river_flow_known=NULL WHERE world_id='fb2a872a-d2dc-4c7f-8d45-05298a430c41'; UPDATE map_tiles SET terrain='plains' WHERE world_id='fb2a872a-d2dc-4c7f-8d45-05298a430c41' AND q BETWEEN 5 AND 11 AND r BETWEEN 25 AND 28; UPDATE map_tiles SET terrain='plains',coastal=true WHERE world_id='fb2a872a-d2dc-4c7f-8d45-05298a430c41' AND q=18 AND r=25; UPDATE map_tiles SET coastal=true WHERE world_id='fb2a872a-d2dc-4c7f-8d45-05298a430c41' AND q=5 AND r=25; UPDATE units SET q=5,r=25,status='positioned' WHERE id='61464117-7845-44e3-844f-bbce75e973e8'; DELETE FROM sea_storms WHERE world_id='fb2a872a-d2dc-4c7f-8d45-05298a430c41';

INSERT INTO units(world_id,owner_id,type,category,size,ordinal,status,q,r) VALUES('fb2a872a-d2dc-4c7f-8d45-05298a430c41','3932afec-da46-419a-99f4-2325fb74c15f','elite_infantry','land',100,1,'positioned',5,25);

INSERT INTO units(world_id,owner_id,type,category,size,ordinal,status,q,r) VALUES('fb2a872a-d2dc-4c7f-8d45-05298a430c41','3932afec-da46-419a-99f4-2325fb74c15f','war_chariot','land',100,1,'positioned',5,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('fb2a872a-d2dc-4c7f-8d45-05298a430c41','3932afec-da46-419a-99f4-2325fb74c15f','merchantman','naval',1,10,'Cargo 0','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('fb2a872a-d2dc-4c7f-8d45-05298a430c41','3932afec-da46-419a-99f4-2325fb74c15f','merchantman','naval',1,10,'Cargo 1','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('fb2a872a-d2dc-4c7f-8d45-05298a430c41','3932afec-da46-419a-99f4-2325fb74c15f','merchantman','naval',1,10,'Cargo 2','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('fb2a872a-d2dc-4c7f-8d45-05298a430c41','3932afec-da46-419a-99f4-2325fb74c15f','merchantman','naval',1,10,'Cargo 3','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('fb2a872a-d2dc-4c7f-8d45-05298a430c41','3932afec-da46-419a-99f4-2325fb74c15f','merchantman','naval',1,10,'Cargo 4','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('fb2a872a-d2dc-4c7f-8d45-05298a430c41','3932afec-da46-419a-99f4-2325fb74c15f','merchantman','naval',1,10,'Cargo 5','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('fb2a872a-d2dc-4c7f-8d45-05298a430c41','3932afec-da46-419a-99f4-2325fb74c15f','merchantman','naval',1,10,'Cargo 6','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('fb2a872a-d2dc-4c7f-8d45-05298a430c41','3932afec-da46-419a-99f4-2325fb74c15f','merchantman','naval',1,10,'Cargo 7','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('fb2a872a-d2dc-4c7f-8d45-05298a430c41','3932afec-da46-419a-99f4-2325fb74c15f','merchantman','naval',1,10,'Cargo 8','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('fb2a872a-d2dc-4c7f-8d45-05298a430c41','3932afec-da46-419a-99f4-2325fb74c15f','merchantman','naval',1,10,'Cargo 9','positioned',4,25);

UPDATE map_tiles SET terrain='coastal_sea',coastal=false,river_edges=NULL,river_upstream_edges=NULL,river_downstream_edges=NULL,river_flow_known=NULL WHERE world_id='fb2a872a-d2dc-4c7f-8d45-05298a430c41'; UPDATE map_tiles SET terrain='plains' WHERE world_id='fb2a872a-d2dc-4c7f-8d45-05298a430c41' AND q BETWEEN 5 AND 17 AND r BETWEEN 22 AND 28; UPDATE map_tiles SET terrain='river' WHERE world_id='fb2a872a-d2dc-4c7f-8d45-05298a430c41' AND q BETWEEN 10 AND 17 AND r=25; UPDATE map_tiles SET terrain='river_ford' WHERE world_id='fb2a872a-d2dc-4c7f-8d45-05298a430c41' AND q=13 AND r=25; UPDATE map_tiles SET terrain='coastal_sea' WHERE world_id='fb2a872a-d2dc-4c7f-8d45-05298a430c41' AND q=9 AND r=25; UPDATE map_tiles SET terrain='river_delta' WHERE world_id='fb2a872a-d2dc-4c7f-8d45-05298a430c41' AND q=17 AND r=26; DELETE FROM player_scouted_tiles WHERE world_id='fb2a872a-d2dc-4c7f-8d45-05298a430c41' AND player_id='3932afec-da46-419a-99f4-2325fb74c15f'; DELETE FROM player_scouted_provinces WHERE world_id='fb2a872a-d2dc-4c7f-8d45-05298a430c41' AND player_id='3932afec-da46-419a-99f4-2325fb74c15f';

UPDATE map_tiles SET river_edges=CASE WHEN q=17 THEN 19 ELSE 3 END,river_upstream_edges=2,river_downstream_edges=CASE WHEN q=17 THEN 17 ELSE 1 END,river_flow_known=true WHERE world_id='fb2a872a-d2dc-4c7f-8d45-05298a430c41' AND terrain IN('river','river_ford')

INSERT INTO units(world_id,owner_id,type,category,size,ordinal,status,q,r) VALUES('fb2a872a-d2dc-4c7f-8d45-05298a430c41','3932afec-da46-419a-99f4-2325fb74c15f','infantry','land',100,2,'positioned',8,26) RETURNING id

UPDATE units SET q=8,r=26 WHERE id='6cc2c0ab-a284-4d82-9b6d-1f81ccaff9e8'

DELETE FROM player_scouted_tiles WHERE world_id='fb2a872a-d2dc-4c7f-8d45-05298a430c41' AND player_id='3932afec-da46-419a-99f4-2325fb74c15f'; INSERT INTO player_scouted_tiles(world_id,player_id,q,r) VALUES('fb2a872a-d2dc-4c7f-8d45-05298a430c41','3932afec-da46-419a-99f4-2325fb74c15f',9,25),('fb2a872a-d2dc-4c7f-8d45-05298a430c41','3932afec-da46-419a-99f4-2325fb74c15f',10,25) ON CONFLICT DO NOTHING

UPDATE units SET q=11,r=26 WHERE id='6cc2c0ab-a284-4d82-9b6d-1f81ccaff9e8'

DELETE FROM player_scouted_tiles WHERE world_id='fb2a872a-d2dc-4c7f-8d45-05298a430c41' AND player_id='3932afec-da46-419a-99f4-2325fb74c15f'; INSERT INTO player_scouted_tiles(world_id,player_id,q,r) VALUES('fb2a872a-d2dc-4c7f-8d45-05298a430c41','3932afec-da46-419a-99f4-2325fb74c15f',13,25),('fb2a872a-d2dc-4c7f-8d45-05298a430c41','3932afec-da46-419a-99f4-2325fb74c15f',12,25) ON CONFLICT DO NOTHING

UPDATE units SET q=15,r=26 WHERE id='6cc2c0ab-a284-4d82-9b6d-1f81ccaff9e8'

DELETE FROM player_scouted_tiles WHERE world_id='fb2a872a-d2dc-4c7f-8d45-05298a430c41' AND player_id='3932afec-da46-419a-99f4-2325fb74c15f'; INSERT INTO player_scouted_tiles(world_id,player_id,q,r) VALUES('fb2a872a-d2dc-4c7f-8d45-05298a430c41','3932afec-da46-419a-99f4-2325fb74c15f',17,26),('fb2a872a-d2dc-4c7f-8d45-05298a430c41','3932afec-da46-419a-99f4-2325fb74c15f',16,25),('fb2a872a-d2dc-4c7f-8d45-05298a430c41','3932afec-da46-419a-99f4-2325fb74c15f',17,25),('fb2a872a-d2dc-4c7f-8d45-05298a430c41','3932afec-da46-419a-99f4-2325fb74c15f',18,25),('fb2a872a-d2dc-4c7f-8d45-05298a430c41','3932afec-da46-419a-99f4-2325fb74c15f',18,24) ON CONFLICT DO NOTHING

UPDATE units SET q=8,r=26 WHERE id='6cc2c0ab-a284-4d82-9b6d-1f81ccaff9e8'

DELETE FROM player_scouted_tiles WHERE world_id='fb2a872a-d2dc-4c7f-8d45-05298a430c41' AND player_id='3932afec-da46-419a-99f4-2325fb74c15f'; INSERT INTO player_scouted_tiles(world_id,player_id,q,r) VALUES('fb2a872a-d2dc-4c7f-8d45-05298a430c41','3932afec-da46-419a-99f4-2325fb74c15f',9,25),('fb2a872a-d2dc-4c7f-8d45-05298a430c41','3932afec-da46-419a-99f4-2325fb74c15f',10,25) ON CONFLICT DO NOTHING

UPDATE units SET q=11,r=26 WHERE id='6cc2c0ab-a284-4d82-9b6d-1f81ccaff9e8'

DELETE FROM player_scouted_tiles WHERE world_id='fb2a872a-d2dc-4c7f-8d45-05298a430c41' AND player_id='3932afec-da46-419a-99f4-2325fb74c15f'; INSERT INTO player_scouted_tiles(world_id,player_id,q,r) VALUES('fb2a872a-d2dc-4c7f-8d45-05298a430c41','3932afec-da46-419a-99f4-2325fb74c15f',13,25),('fb2a872a-d2dc-4c7f-8d45-05298a430c41','3932afec-da46-419a-99f4-2325fb74c15f',12,25) ON CONFLICT DO NOTHING

UPDATE units SET q=15,r=26 WHERE id='6cc2c0ab-a284-4d82-9b6d-1f81ccaff9e8'

DELETE FROM player_scouted_tiles WHERE world_id='fb2a872a-d2dc-4c7f-8d45-05298a430c41' AND player_id='3932afec-da46-419a-99f4-2325fb74c15f'; INSERT INTO player_scouted_tiles(world_id,player_id,q,r) VALUES('fb2a872a-d2dc-4c7f-8d45-05298a430c41','3932afec-da46-419a-99f4-2325fb74c15f',17,26),('fb2a872a-d2dc-4c7f-8d45-05298a430c41','3932afec-da46-419a-99f4-2325fb74c15f',16,25),('fb2a872a-d2dc-4c7f-8d45-05298a430c41','3932afec-da46-419a-99f4-2325fb74c15f',17,25),('fb2a872a-d2dc-4c7f-8d45-05298a430c41','3932afec-da46-419a-99f4-2325fb74c15f',18,25),('fb2a872a-d2dc-4c7f-8d45-05298a430c41','3932afec-da46-419a-99f4-2325fb74c15f',18,24) ON CONFLICT DO NOTHING
