UPDATE map_tiles SET terrain='coastal_sea',coastal=false WHERE world_id='03723084-fb6f-413b-b15b-a3438b9b670e'; UPDATE map_tiles SET terrain='plains' WHERE world_id='03723084-fb6f-413b-b15b-a3438b9b670e' AND q BETWEEN 5 AND 11 AND r BETWEEN 25 AND 28; UPDATE map_tiles SET terrain='plains',coastal=true WHERE world_id='03723084-fb6f-413b-b15b-a3438b9b670e' AND q=18 AND r=25; UPDATE map_tiles SET coastal=true WHERE world_id='03723084-fb6f-413b-b15b-a3438b9b670e' AND q=5 AND r=25; UPDATE units SET q=5,r=25,status='positioned' WHERE id='135976c1-60b4-48d9-b13f-8f769fb5c793'; DELETE FROM sea_storms WHERE world_id='03723084-fb6f-413b-b15b-a3438b9b670e';

INSERT INTO units(world_id,owner_id,type,category,size,ordinal,status,q,r) VALUES('03723084-fb6f-413b-b15b-a3438b9b670e','31c13aad-d2bd-411c-8185-f75fa3ce4fb5','elite_infantry','land',100,1,'positioned',5,25);

INSERT INTO units(world_id,owner_id,type,category,size,ordinal,status,q,r) VALUES('03723084-fb6f-413b-b15b-a3438b9b670e','31c13aad-d2bd-411c-8185-f75fa3ce4fb5','war_chariot','land',100,1,'positioned',5,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('03723084-fb6f-413b-b15b-a3438b9b670e','31c13aad-d2bd-411c-8185-f75fa3ce4fb5','merchantman','naval',1,10,'Cargo 0','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('03723084-fb6f-413b-b15b-a3438b9b670e','31c13aad-d2bd-411c-8185-f75fa3ce4fb5','merchantman','naval',1,10,'Cargo 1','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('03723084-fb6f-413b-b15b-a3438b9b670e','31c13aad-d2bd-411c-8185-f75fa3ce4fb5','merchantman','naval',1,10,'Cargo 2','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('03723084-fb6f-413b-b15b-a3438b9b670e','31c13aad-d2bd-411c-8185-f75fa3ce4fb5','merchantman','naval',1,10,'Cargo 3','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('03723084-fb6f-413b-b15b-a3438b9b670e','31c13aad-d2bd-411c-8185-f75fa3ce4fb5','merchantman','naval',1,10,'Cargo 4','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('03723084-fb6f-413b-b15b-a3438b9b670e','31c13aad-d2bd-411c-8185-f75fa3ce4fb5','merchantman','naval',1,10,'Cargo 5','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('03723084-fb6f-413b-b15b-a3438b9b670e','31c13aad-d2bd-411c-8185-f75fa3ce4fb5','merchantman','naval',1,10,'Cargo 6','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('03723084-fb6f-413b-b15b-a3438b9b670e','31c13aad-d2bd-411c-8185-f75fa3ce4fb5','merchantman','naval',1,10,'Cargo 7','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('03723084-fb6f-413b-b15b-a3438b9b670e','31c13aad-d2bd-411c-8185-f75fa3ce4fb5','merchantman','naval',1,10,'Cargo 8','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('03723084-fb6f-413b-b15b-a3438b9b670e','31c13aad-d2bd-411c-8185-f75fa3ce4fb5','merchantman','naval',1,10,'Cargo 9','positioned',4,25);

UPDATE provinces SET coastal=true WHERE id='cce05cac-6a9f-4dff-b022-95df99649f9c'; UPDATE settlements SET population=4000 WHERE id='6fec639d-a023-4104-884d-af535e347ee4'; INSERT INTO buildings(settlement_id,building_type,level) VALUES('6fec639d-a023-4104-884d-af535e347ee4','harbour',1) ON CONFLICT DO NOTHING; UPDATE units SET settlement_id='6fec639d-a023-4104-884d-af535e347ee4',support_settlement_id='6fec639d-a023-4104-884d-af535e347ee4',status='garrison',q=NULL,r=NULL WHERE world_id='03723084-fb6f-413b-b15b-a3438b9b670e' AND owner_id='31c13aad-d2bd-411c-8185-f75fa3ce4fb5' AND type<>'nomadic_host' AND status<>'disbanded';

INSERT INTO provinces(world_id,map_q,map_r,terrain_type,coastal) VALUES('03723084-fb6f-413b-b15b-a3438b9b670e',10,25,'plains',true) RETURNING id;

UPDATE provinces SET terrain_type='plains',coastal=true WHERE id='5cbfa806-4cd9-48e9-92a1-2a8980193e67';

INSERT INTO settlements(world_id,province_id,name,culture_id,owner_id,control_type,state,population) VALUES('03723084-fb6f-413b-b15b-a3438b9b670e','5cbfa806-4cd9-48e9-92a1-2a8980193e67','Petras','minoan','31c13aad-d2bd-411c-8185-f75fa3ce4fb5','colony','active',2000) RETURNING id;

INSERT INTO buildings(settlement_id,building_type,level) VALUES('d9ccc4f9-b35a-4fa2-86e7-9de6859b2456','harbour',1);

INSERT INTO provinces(world_id,map_q,map_r,terrain_type,coastal) VALUES('03723084-fb6f-413b-b15b-a3438b9b670e',18,25,'plains',true) RETURNING id;

UPDATE provinces SET terrain_type='plains',coastal=true WHERE id='cae6c61d-1092-4d3b-a2ca-465db9259946';

INSERT INTO settlements(world_id,province_id,name,culture_id,owner_id,control_type,state,population) VALUES('03723084-fb6f-413b-b15b-a3438b9b670e','cae6c61d-1092-4d3b-a2ca-465db9259946','Amnissos','minoan','31c13aad-d2bd-411c-8185-f75fa3ce4fb5','colony','active',2000) RETURNING id;

INSERT INTO buildings(settlement_id,building_type,level) VALUES('ab1b85cb-ca47-41b6-9fee-351d4d17544c','harbour',1);

INSERT INTO provinces(world_id,map_q,map_r,terrain_type,coastal) VALUES('03723084-fb6f-413b-b15b-a3438b9b670e',7,28,'plains',false) RETURNING id;

UPDATE provinces SET terrain_type='plains',coastal=false WHERE id='13adf96f-36d5-4313-80e4-bc0f28e8c07f';

INSERT INTO settlements(world_id,province_id,name,culture_id,owner_id,control_type,state,population) VALUES('03723084-fb6f-413b-b15b-a3438b9b670e','13adf96f-36d5-4313-80e4-bc0f28e8c07f','Inland','minoan','31c13aad-d2bd-411c-8185-f75fa3ce4fb5','colony','active',2000) RETURNING id;

INSERT INTO provinces(world_id,map_q,map_r,terrain_type,coastal) VALUES('03723084-fb6f-413b-b15b-a3438b9b670e',10,28,'plains',true) RETURNING id;

UPDATE provinces SET terrain_type='plains',coastal=true WHERE id='f9e36079-c894-45b6-bf8e-1ebf95c861fe';

INSERT INTO settlements(world_id,province_id,name,culture_id,owner_id,control_type,state,population) VALUES('03723084-fb6f-413b-b15b-a3438b9b670e','f9e36079-c894-45b6-bf8e-1ebf95c861fe','Kyme','minoan','31c13aad-d2bd-411c-8185-f75fa3ce4fb5','colony','active',2000) RETURNING id;

INSERT INTO buildings(settlement_id,building_type,level) VALUES('96f1f46c-84be-4630-b7ae-7b03f5ff1084','harbour',1);

INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) SELECT '6fec639d-a023-4104-884d-af535e347ee4',key,10000,0,1000000,current_world_tick() FROM goods ON CONFLICT(settlement_id,good_key) DO UPDATE SET amount=10000,rate=0,cap=1000000,calc_tick=current_world_tick();

INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) SELECT 'd9ccc4f9-b35a-4fa2-86e7-9de6859b2456',key,10000,0,1000000,current_world_tick() FROM goods ON CONFLICT(settlement_id,good_key) DO UPDATE SET amount=10000,rate=0,cap=1000000,calc_tick=current_world_tick();

INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) SELECT 'ab1b85cb-ca47-41b6-9fee-351d4d17544c',key,10000,0,1000000,current_world_tick() FROM goods ON CONFLICT(settlement_id,good_key) DO UPDATE SET amount=10000,rate=0,cap=1000000,calc_tick=current_world_tick();

INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) SELECT '6551a96f-0a43-4497-99a8-4b21d619a6c0',key,10000,0,1000000,current_world_tick() FROM goods ON CONFLICT(settlement_id,good_key) DO UPDATE SET amount=10000,rate=0,cap=1000000,calc_tick=current_world_tick();

INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) SELECT '96f1f46c-84be-4630-b7ae-7b03f5ff1084',key,10000,0,1000000,current_world_tick() FROM goods ON CONFLICT(settlement_id,good_key) DO UPDATE SET amount=10000,rate=0,cap=1000000,calc_tick=current_world_tick();

UPDATE settlements SET owner_id='3b277fd3-0c08-4d8a-a793-804eb4019e6f' WHERE id='96f1f46c-84be-4630-b7ae-7b03f5ff1084'; INSERT INTO player_scouted_tiles(world_id,player_id,q,r) VALUES('03723084-fb6f-413b-b15b-a3438b9b670e','31c13aad-d2bd-411c-8185-f75fa3ce4fb5',10,28) ON CONFLICT DO NOTHING;

INSERT INTO player_scouted_tiles(world_id,player_id,q,r) VALUES('03723084-fb6f-413b-b15b-a3438b9b670e','3b277fd3-0c08-4d8a-a793-804eb4019e6f',5,25) ON CONFLICT DO NOTHING

UPDATE worlds SET current_tick=4,last_tick_at=now() WHERE id='03723084-fb6f-413b-b15b-a3438b9b670e'; UPDATE scheduled_events SET process_after=now() WHERE world_id='03723084-fb6f-413b-b15b-a3438b9b670e' AND event_type IN('MessengerArrival','MessengerArrivalV2');
