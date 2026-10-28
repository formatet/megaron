UPDATE map_tiles SET terrain='coastal_sea',coastal=false,river_edges=NULL,river_upstream_edges=NULL,river_downstream_edges=NULL,river_flow_known=NULL WHERE world_id='ca7574cd-814c-4a07-8d41-0dced90def7f'; UPDATE map_tiles SET terrain='plains' WHERE world_id='ca7574cd-814c-4a07-8d41-0dced90def7f' AND q BETWEEN 5 AND 11 AND r BETWEEN 25 AND 28; UPDATE map_tiles SET terrain='plains',coastal=true WHERE world_id='ca7574cd-814c-4a07-8d41-0dced90def7f' AND q=18 AND r=25; UPDATE map_tiles SET coastal=true WHERE world_id='ca7574cd-814c-4a07-8d41-0dced90def7f' AND q=5 AND r=25; UPDATE units SET q=5,r=25,status='positioned' WHERE id='f3550d33-6918-4f30-84ad-ad3043948891'; DELETE FROM sea_storms WHERE world_id='ca7574cd-814c-4a07-8d41-0dced90def7f';

INSERT INTO units(world_id,owner_id,type,category,size,ordinal,status,q,r) VALUES('ca7574cd-814c-4a07-8d41-0dced90def7f','4a339572-a379-43ea-a111-187ed6e3293a','elite_infantry','land',100,1,'positioned',5,25);

INSERT INTO units(world_id,owner_id,type,category,size,ordinal,status,q,r) VALUES('ca7574cd-814c-4a07-8d41-0dced90def7f','4a339572-a379-43ea-a111-187ed6e3293a','war_chariot','land',100,1,'positioned',5,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('ca7574cd-814c-4a07-8d41-0dced90def7f','4a339572-a379-43ea-a111-187ed6e3293a','merchantman','naval',1,10,'Cargo 0','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('ca7574cd-814c-4a07-8d41-0dced90def7f','4a339572-a379-43ea-a111-187ed6e3293a','merchantman','naval',1,10,'Cargo 1','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('ca7574cd-814c-4a07-8d41-0dced90def7f','4a339572-a379-43ea-a111-187ed6e3293a','merchantman','naval',1,10,'Cargo 2','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('ca7574cd-814c-4a07-8d41-0dced90def7f','4a339572-a379-43ea-a111-187ed6e3293a','merchantman','naval',1,10,'Cargo 3','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('ca7574cd-814c-4a07-8d41-0dced90def7f','4a339572-a379-43ea-a111-187ed6e3293a','merchantman','naval',1,10,'Cargo 4','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('ca7574cd-814c-4a07-8d41-0dced90def7f','4a339572-a379-43ea-a111-187ed6e3293a','merchantman','naval',1,10,'Cargo 5','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('ca7574cd-814c-4a07-8d41-0dced90def7f','4a339572-a379-43ea-a111-187ed6e3293a','merchantman','naval',1,10,'Cargo 6','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('ca7574cd-814c-4a07-8d41-0dced90def7f','4a339572-a379-43ea-a111-187ed6e3293a','merchantman','naval',1,10,'Cargo 7','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('ca7574cd-814c-4a07-8d41-0dced90def7f','4a339572-a379-43ea-a111-187ed6e3293a','merchantman','naval',1,10,'Cargo 8','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('ca7574cd-814c-4a07-8d41-0dced90def7f','4a339572-a379-43ea-a111-187ed6e3293a','merchantman','naval',1,10,'Cargo 9','positioned',4,25);

UPDATE map_tiles SET terrain='coastal_sea',coastal=false,river_edges=NULL,river_upstream_edges=NULL,river_downstream_edges=NULL,river_flow_known=NULL WHERE world_id='ca7574cd-814c-4a07-8d41-0dced90def7f'; UPDATE map_tiles SET terrain='plains' WHERE world_id='ca7574cd-814c-4a07-8d41-0dced90def7f' AND q BETWEEN 5 AND 17 AND r BETWEEN 25 AND 29; UPDATE units SET status='disbanded' WHERE world_id='ca7574cd-814c-4a07-8d41-0dced90def7f' AND owner_id='4a339572-a379-43ea-a111-187ed6e3293a'; UPDATE settlements SET population=4000 WHERE id='a533c87b-fa43-4fc0-b98e-265def86b672'; UPDATE settlement_goods SET amount=10000,rate=0,cap=1000000,calc_tick=current_world_tick() WHERE settlement_id='a533c87b-fa43-4fc0-b98e-265def86b672';

INSERT INTO units(world_id,owner_id,type,category,size,crew,hull,name,status,q,r,support_settlement_id,provisions,provision_goods) VALUES('ca7574cd-814c-4a07-8d41-0dced90def7f','4a339572-a379-43ea-a111-187ed6e3293a','merchantman','naval',1,10,5,'White Dolphin','positioned',9,24,'a533c87b-fa43-4fc0-b98e-265def86b672',100,'{"fish":100}') RETURNING id

INSERT INTO units(world_id,owner_id,type,category,size,status,carried_by_ship_id,home_settlement_id,support_settlement_id) VALUES('ca7574cd-814c-4a07-8d41-0dced90def7f','4a339572-a379-43ea-a111-187ed6e3293a','spearman','land',100,'embarked','f6205746-f8bd-4d1b-939e-b05b5866cde6','a533c87b-fa43-4fc0-b98e-265def86b672','a533c87b-fa43-4fc0-b98e-265def86b672') RETURNING id

INSERT INTO units(world_id,owner_id,type,category,size,crew,hull,name,status,q,r) VALUES('ca7574cd-814c-4a07-8d41-0dced90def7f','c87578d1-8141-4653-9f33-1196f53dd6ce','galley','naval',1,30,3,'Red Sail','positioned',11,24) RETURNING id

INSERT INTO units(world_id,owner_id,type,category,size,status,q,r,home_settlement_id,support_settlement_id) VALUES('ca7574cd-814c-4a07-8d41-0dced90def7f','4a339572-a379-43ea-a111-187ed6e3293a','spearman','land',100,'positioned',10,25,'a533c87b-fa43-4fc0-b98e-265def86b672','a533c87b-fa43-4fc0-b98e-265def86b672') RETURNING id

INSERT INTO units(world_id,owner_id,type,category,size,crew,hull,name,status,q,r,support_settlement_id) VALUES('ca7574cd-814c-4a07-8d41-0dced90def7f','4a339572-a379-43ea-a111-187ed6e3293a','galley','naval',1,30,4,'Blue Dolphin','positioned',13,24,'a533c87b-fa43-4fc0-b98e-265def86b672') RETURNING id

INSERT INTO units(world_id,owner_id,type,category,size,crew,hull,name,status,q,r,support_settlement_id) VALUES('ca7574cd-814c-4a07-8d41-0dced90def7f','4a339572-a379-43ea-a111-187ed6e3293a','galley','naval',1,30,4,'Bronze Sail','positioned',13,24,'a533c87b-fa43-4fc0-b98e-265def86b672') RETURNING id

INSERT INTO units(world_id,owner_id,type,category,size,ordinal,status,q,r,target_q,target_r,march_route,depart_tick,arrive_tick,departs_at,arrives_at,home_settlement_id,support_settlement_id) VALUES('ca7574cd-814c-4a07-8d41-0dced90def7f','4a339572-a379-43ea-a111-187ed6e3293a','spearman','land',100,3,'marching',7,27,11,27,'{"start_tick": -2, "end_tick": 2, "hexes": [[7, 27], [8, 27], [9, 27], [10, 27], [11, 27]], "costs": [1000, 1000, 1000, 1000]}',-2,2,'2026-10-28T14:07:21.052614+00:00','2026-10-28T18:07:21.052614+00:00','a533c87b-fa43-4fc0-b98e-265def86b672','a533c87b-fa43-4fc0-b98e-265def86b672') RETURNING id

INSERT INTO player_scouted_tiles(world_id,player_id,q,r) SELECT world_id,'4a339572-a379-43ea-a111-187ed6e3293a',q,r FROM map_tiles WHERE world_id='ca7574cd-814c-4a07-8d41-0dced90def7f' AND q BETWEEN 5 AND 17 AND r BETWEEN 23 AND 29 ON CONFLICT DO NOTHING

INSERT INTO units(world_id,owner_id,type,category,size,crew,hull,name,status,q,r) VALUES('ca7574cd-814c-4a07-8d41-0dced90def7f','4a339572-a379-43ea-a111-187ed6e3293a','merchantman','naval',1,10,5,'Third Sail','positioned',13,24) RETURNING id
