UPDATE map_tiles SET terrain='coastal_sea',coastal=false,river_edges=NULL,river_upstream_edges=NULL,river_downstream_edges=NULL,river_flow_known=NULL WHERE world_id='998caa07-678d-433a-b3a7-2fd28d78acf0'; UPDATE map_tiles SET terrain='plains' WHERE world_id='998caa07-678d-433a-b3a7-2fd28d78acf0' AND q BETWEEN 5 AND 11 AND r BETWEEN 25 AND 28; UPDATE map_tiles SET terrain='plains',coastal=true WHERE world_id='998caa07-678d-433a-b3a7-2fd28d78acf0' AND q=18 AND r=25; UPDATE map_tiles SET coastal=true WHERE world_id='998caa07-678d-433a-b3a7-2fd28d78acf0' AND q=5 AND r=25; UPDATE units SET q=5,r=25,status='positioned' WHERE id='9b403fb1-2196-44f2-a46b-96c139062a09'; DELETE FROM sea_storms WHERE world_id='998caa07-678d-433a-b3a7-2fd28d78acf0';

INSERT INTO units(world_id,owner_id,type,category,size,ordinal,status,q,r) VALUES('998caa07-678d-433a-b3a7-2fd28d78acf0','10c83e0f-7327-446b-8586-24564becc6b6','elite_infantry','land',100,1,'positioned',5,25);

INSERT INTO units(world_id,owner_id,type,category,size,ordinal,status,q,r) VALUES('998caa07-678d-433a-b3a7-2fd28d78acf0','10c83e0f-7327-446b-8586-24564becc6b6','war_chariot','land',100,1,'positioned',5,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('998caa07-678d-433a-b3a7-2fd28d78acf0','10c83e0f-7327-446b-8586-24564becc6b6','merchantman','naval',1,10,'Cargo 0','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('998caa07-678d-433a-b3a7-2fd28d78acf0','10c83e0f-7327-446b-8586-24564becc6b6','merchantman','naval',1,10,'Cargo 1','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('998caa07-678d-433a-b3a7-2fd28d78acf0','10c83e0f-7327-446b-8586-24564becc6b6','merchantman','naval',1,10,'Cargo 2','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('998caa07-678d-433a-b3a7-2fd28d78acf0','10c83e0f-7327-446b-8586-24564becc6b6','merchantman','naval',1,10,'Cargo 3','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('998caa07-678d-433a-b3a7-2fd28d78acf0','10c83e0f-7327-446b-8586-24564becc6b6','merchantman','naval',1,10,'Cargo 4','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('998caa07-678d-433a-b3a7-2fd28d78acf0','10c83e0f-7327-446b-8586-24564becc6b6','merchantman','naval',1,10,'Cargo 5','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('998caa07-678d-433a-b3a7-2fd28d78acf0','10c83e0f-7327-446b-8586-24564becc6b6','merchantman','naval',1,10,'Cargo 6','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('998caa07-678d-433a-b3a7-2fd28d78acf0','10c83e0f-7327-446b-8586-24564becc6b6','merchantman','naval',1,10,'Cargo 7','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('998caa07-678d-433a-b3a7-2fd28d78acf0','10c83e0f-7327-446b-8586-24564becc6b6','merchantman','naval',1,10,'Cargo 8','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('998caa07-678d-433a-b3a7-2fd28d78acf0','10c83e0f-7327-446b-8586-24564becc6b6','merchantman','naval',1,10,'Cargo 9','positioned',4,25);

UPDATE map_tiles SET terrain='coastal_sea',coastal=false,river_edges=NULL,river_upstream_edges=NULL,river_downstream_edges=NULL,river_flow_known=NULL WHERE world_id='998caa07-678d-433a-b3a7-2fd28d78acf0'; UPDATE map_tiles SET terrain='plains' WHERE world_id='998caa07-678d-433a-b3a7-2fd28d78acf0' AND q BETWEEN 5 AND 17 AND r BETWEEN 25 AND 29; UPDATE units SET status='disbanded' WHERE world_id='998caa07-678d-433a-b3a7-2fd28d78acf0' AND owner_id='10c83e0f-7327-446b-8586-24564becc6b6'; UPDATE settlements SET population=4000 WHERE id='edb83ab2-09e5-4cb2-95ef-f8294076552f'; UPDATE settlement_goods SET amount=10000,rate=0,cap=1000000,calc_tick=current_world_tick() WHERE settlement_id='edb83ab2-09e5-4cb2-95ef-f8294076552f';

INSERT INTO units(world_id,owner_id,type,category,size,crew,hull,name,status,q,r,support_settlement_id,provisions,provision_goods) VALUES('998caa07-678d-433a-b3a7-2fd28d78acf0','10c83e0f-7327-446b-8586-24564becc6b6','merchantman','naval',1,10,5,'White Dolphin','positioned',9,24,'edb83ab2-09e5-4cb2-95ef-f8294076552f',100,'{"fish":100}') RETURNING id

INSERT INTO units(world_id,owner_id,type,category,size,status,carried_by_ship_id,home_settlement_id,support_settlement_id) VALUES('998caa07-678d-433a-b3a7-2fd28d78acf0','10c83e0f-7327-446b-8586-24564becc6b6','spearman','land',100,'embarked','7a32c1db-e987-4911-b06c-d6ed8ada874d','edb83ab2-09e5-4cb2-95ef-f8294076552f','edb83ab2-09e5-4cb2-95ef-f8294076552f') RETURNING id

INSERT INTO units(world_id,owner_id,type,category,size,crew,hull,name,status,q,r) VALUES('998caa07-678d-433a-b3a7-2fd28d78acf0','fbb43f72-73be-474d-bcbd-bacccb1b960b','galley','naval',1,30,3,'Red Sail','positioned',11,24) RETURNING id

INSERT INTO units(world_id,owner_id,type,category,size,status,q,r,home_settlement_id,support_settlement_id) VALUES('998caa07-678d-433a-b3a7-2fd28d78acf0','10c83e0f-7327-446b-8586-24564becc6b6','spearman','land',100,'positioned',10,25,'edb83ab2-09e5-4cb2-95ef-f8294076552f','edb83ab2-09e5-4cb2-95ef-f8294076552f') RETURNING id

INSERT INTO units(world_id,owner_id,type,category,size,crew,hull,name,status,q,r,support_settlement_id) VALUES('998caa07-678d-433a-b3a7-2fd28d78acf0','10c83e0f-7327-446b-8586-24564becc6b6','galley','naval',1,30,4,'Blue Dolphin','positioned',13,24,'edb83ab2-09e5-4cb2-95ef-f8294076552f') RETURNING id

INSERT INTO units(world_id,owner_id,type,category,size,crew,hull,name,status,q,r,support_settlement_id) VALUES('998caa07-678d-433a-b3a7-2fd28d78acf0','10c83e0f-7327-446b-8586-24564becc6b6','galley','naval',1,30,4,'Bronze Sail','positioned',13,24,'edb83ab2-09e5-4cb2-95ef-f8294076552f') RETURNING id

INSERT INTO units(world_id,owner_id,type,category,size,ordinal,status,q,r,target_q,target_r,march_route,depart_tick,arrive_tick,departs_at,arrives_at,home_settlement_id,support_settlement_id) VALUES('998caa07-678d-433a-b3a7-2fd28d78acf0','10c83e0f-7327-446b-8586-24564becc6b6','spearman','land',100,3,'marching',7,27,11,27,'{"start_tick": -2, "end_tick": 2, "hexes": [[7, 27], [8, 27], [9, 27], [10, 27], [11, 27]], "costs": [1000, 1000, 1000, 1000]}',-2,2,'2026-10-28T14:08:21.903272+00:00','2026-10-28T18:08:21.903272+00:00','edb83ab2-09e5-4cb2-95ef-f8294076552f','edb83ab2-09e5-4cb2-95ef-f8294076552f') RETURNING id

INSERT INTO player_scouted_tiles(world_id,player_id,q,r) SELECT world_id,'10c83e0f-7327-446b-8586-24564becc6b6',q,r FROM map_tiles WHERE world_id='998caa07-678d-433a-b3a7-2fd28d78acf0' AND q BETWEEN 5 AND 17 AND r BETWEEN 23 AND 29 ON CONFLICT DO NOTHING

INSERT INTO units(world_id,owner_id,type,category,size,crew,hull,name,status,q,r) VALUES('998caa07-678d-433a-b3a7-2fd28d78acf0','10c83e0f-7327-446b-8586-24564becc6b6','merchantman','naval',1,10,5,'Third Sail','positioned',13,24) RETURNING id
