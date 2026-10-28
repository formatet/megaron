UPDATE map_tiles SET terrain='coastal_sea',coastal=false,river_edges=NULL,river_upstream_edges=NULL,river_downstream_edges=NULL,river_flow_known=NULL WHERE world_id='fa9f9bbf-efec-4715-b618-86734c7126b0'; UPDATE map_tiles SET terrain='plains' WHERE world_id='fa9f9bbf-efec-4715-b618-86734c7126b0' AND q BETWEEN 5 AND 11 AND r BETWEEN 25 AND 28; UPDATE map_tiles SET terrain='plains',coastal=true WHERE world_id='fa9f9bbf-efec-4715-b618-86734c7126b0' AND q=18 AND r=25; UPDATE map_tiles SET coastal=true WHERE world_id='fa9f9bbf-efec-4715-b618-86734c7126b0' AND q=5 AND r=25; UPDATE units SET q=5,r=25,status='positioned' WHERE id='ed52645e-af71-4ac5-84f9-f600138b1233'; DELETE FROM sea_storms WHERE world_id='fa9f9bbf-efec-4715-b618-86734c7126b0';

INSERT INTO units(world_id,owner_id,type,category,size,ordinal,status,q,r) VALUES('fa9f9bbf-efec-4715-b618-86734c7126b0','8bf1774e-0c5d-48cd-aa63-807ac9050cd6','elite_infantry','land',100,1,'positioned',5,25);

INSERT INTO units(world_id,owner_id,type,category,size,ordinal,status,q,r) VALUES('fa9f9bbf-efec-4715-b618-86734c7126b0','8bf1774e-0c5d-48cd-aa63-807ac9050cd6','war_chariot','land',100,1,'positioned',5,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('fa9f9bbf-efec-4715-b618-86734c7126b0','8bf1774e-0c5d-48cd-aa63-807ac9050cd6','merchantman','naval',1,10,'Cargo 0','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('fa9f9bbf-efec-4715-b618-86734c7126b0','8bf1774e-0c5d-48cd-aa63-807ac9050cd6','merchantman','naval',1,10,'Cargo 1','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('fa9f9bbf-efec-4715-b618-86734c7126b0','8bf1774e-0c5d-48cd-aa63-807ac9050cd6','merchantman','naval',1,10,'Cargo 2','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('fa9f9bbf-efec-4715-b618-86734c7126b0','8bf1774e-0c5d-48cd-aa63-807ac9050cd6','merchantman','naval',1,10,'Cargo 3','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('fa9f9bbf-efec-4715-b618-86734c7126b0','8bf1774e-0c5d-48cd-aa63-807ac9050cd6','merchantman','naval',1,10,'Cargo 4','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('fa9f9bbf-efec-4715-b618-86734c7126b0','8bf1774e-0c5d-48cd-aa63-807ac9050cd6','merchantman','naval',1,10,'Cargo 5','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('fa9f9bbf-efec-4715-b618-86734c7126b0','8bf1774e-0c5d-48cd-aa63-807ac9050cd6','merchantman','naval',1,10,'Cargo 6','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('fa9f9bbf-efec-4715-b618-86734c7126b0','8bf1774e-0c5d-48cd-aa63-807ac9050cd6','merchantman','naval',1,10,'Cargo 7','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('fa9f9bbf-efec-4715-b618-86734c7126b0','8bf1774e-0c5d-48cd-aa63-807ac9050cd6','merchantman','naval',1,10,'Cargo 8','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('fa9f9bbf-efec-4715-b618-86734c7126b0','8bf1774e-0c5d-48cd-aa63-807ac9050cd6','merchantman','naval',1,10,'Cargo 9','positioned',4,25);

UPDATE map_tiles SET terrain='coastal_sea',coastal=false,river_edges=NULL,river_upstream_edges=NULL,river_downstream_edges=NULL,river_flow_known=NULL WHERE world_id='fa9f9bbf-efec-4715-b618-86734c7126b0'; UPDATE map_tiles SET terrain='plains' WHERE world_id='fa9f9bbf-efec-4715-b618-86734c7126b0' AND q BETWEEN 5 AND 17 AND r BETWEEN 25 AND 29; UPDATE units SET status='disbanded' WHERE world_id='fa9f9bbf-efec-4715-b618-86734c7126b0' AND owner_id='8bf1774e-0c5d-48cd-aa63-807ac9050cd6'; UPDATE settlements SET population=4000 WHERE id='98d77d28-29f9-4913-a18d-4acb0e72063b'; UPDATE settlement_goods SET amount=10000,rate=0,cap=1000000,calc_tick=current_world_tick() WHERE settlement_id='98d77d28-29f9-4913-a18d-4acb0e72063b';

INSERT INTO units(world_id,owner_id,type,category,size,crew,hull,name,status,q,r,support_settlement_id,provisions,provision_goods) VALUES('fa9f9bbf-efec-4715-b618-86734c7126b0','8bf1774e-0c5d-48cd-aa63-807ac9050cd6','merchantman','naval',1,10,5,'White Dolphin','positioned',9,24,'98d77d28-29f9-4913-a18d-4acb0e72063b',100,'{"fish":100}') RETURNING id

INSERT INTO units(world_id,owner_id,type,category,size,status,carried_by_ship_id,home_settlement_id,support_settlement_id) VALUES('fa9f9bbf-efec-4715-b618-86734c7126b0','8bf1774e-0c5d-48cd-aa63-807ac9050cd6','spearman','land',100,'embarked','d070fec1-39d0-43bf-976b-cef8edb33433','98d77d28-29f9-4913-a18d-4acb0e72063b','98d77d28-29f9-4913-a18d-4acb0e72063b') RETURNING id

INSERT INTO units(world_id,owner_id,type,category,size,crew,hull,name,status,q,r) VALUES('fa9f9bbf-efec-4715-b618-86734c7126b0','0b2b29e8-5bf2-4356-8387-3868c48678ea','galley','naval',1,30,3,'Red Sail','positioned',11,24) RETURNING id

INSERT INTO units(world_id,owner_id,type,category,size,status,q,r,home_settlement_id,support_settlement_id) VALUES('fa9f9bbf-efec-4715-b618-86734c7126b0','8bf1774e-0c5d-48cd-aa63-807ac9050cd6','spearman','land',100,'positioned',10,25,'98d77d28-29f9-4913-a18d-4acb0e72063b','98d77d28-29f9-4913-a18d-4acb0e72063b') RETURNING id

INSERT INTO units(world_id,owner_id,type,category,size,crew,hull,name,status,q,r,support_settlement_id) VALUES('fa9f9bbf-efec-4715-b618-86734c7126b0','8bf1774e-0c5d-48cd-aa63-807ac9050cd6','galley','naval',1,30,4,'Blue Dolphin','positioned',13,24,'98d77d28-29f9-4913-a18d-4acb0e72063b') RETURNING id

INSERT INTO units(world_id,owner_id,type,category,size,crew,hull,name,status,q,r,support_settlement_id) VALUES('fa9f9bbf-efec-4715-b618-86734c7126b0','8bf1774e-0c5d-48cd-aa63-807ac9050cd6','galley','naval',1,30,4,'Bronze Sail','positioned',13,24,'98d77d28-29f9-4913-a18d-4acb0e72063b') RETURNING id

INSERT INTO units(world_id,owner_id,type,category,size,ordinal,status,q,r,target_q,target_r,march_route,depart_tick,arrive_tick,departs_at,arrives_at,home_settlement_id,support_settlement_id) VALUES('fa9f9bbf-efec-4715-b618-86734c7126b0','8bf1774e-0c5d-48cd-aa63-807ac9050cd6','spearman','land',100,3,'marching',7,27,11,27,'{"start_tick": -2, "end_tick": 2, "hexes": [[7, 27], [8, 27], [9, 27], [10, 27], [11, 27]], "costs": [1000, 1000, 1000, 1000]}',-2,2,'2026-10-28T14:07:13.261535+00:00','2026-10-28T18:07:13.261535+00:00','98d77d28-29f9-4913-a18d-4acb0e72063b','98d77d28-29f9-4913-a18d-4acb0e72063b') RETURNING id

INSERT INTO player_scouted_tiles(world_id,player_id,q,r) SELECT world_id,'8bf1774e-0c5d-48cd-aa63-807ac9050cd6',q,r FROM map_tiles WHERE world_id='fa9f9bbf-efec-4715-b618-86734c7126b0' AND q BETWEEN 5 AND 17 AND r BETWEEN 23 AND 29 ON CONFLICT DO NOTHING

INSERT INTO units(world_id,owner_id,type,category,size,crew,hull,name,status,q,r) VALUES('fa9f9bbf-efec-4715-b618-86734c7126b0','8bf1774e-0c5d-48cd-aa63-807ac9050cd6','merchantman','naval',1,10,5,'Third Sail','positioned',13,24) RETURNING id
