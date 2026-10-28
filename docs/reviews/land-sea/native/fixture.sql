UPDATE map_tiles SET terrain='coastal_sea',coastal=false WHERE world_id='30258f82-9283-4b8e-af57-7c3421bf0fa5'; UPDATE map_tiles SET terrain='plains' WHERE world_id='30258f82-9283-4b8e-af57-7c3421bf0fa5' AND q BETWEEN 5 AND 11 AND r BETWEEN 25 AND 28; UPDATE map_tiles SET terrain='plains',coastal=true WHERE world_id='30258f82-9283-4b8e-af57-7c3421bf0fa5' AND q=18 AND r=25; UPDATE map_tiles SET coastal=true WHERE world_id='30258f82-9283-4b8e-af57-7c3421bf0fa5' AND q=5 AND r=25; UPDATE units SET q=5,r=25,status='positioned' WHERE id='d34beb64-c5f3-45e8-8ab3-554b67e0a0e6'; DELETE FROM sea_storms WHERE world_id='30258f82-9283-4b8e-af57-7c3421bf0fa5';

INSERT INTO units(world_id,owner_id,type,category,size,ordinal,status,q,r) VALUES('30258f82-9283-4b8e-af57-7c3421bf0fa5','1edb96af-854d-47b8-8683-9a0f7429af4a','elite_infantry','land',100,1,'positioned',5,25);

INSERT INTO units(world_id,owner_id,type,category,size,ordinal,status,q,r) VALUES('30258f82-9283-4b8e-af57-7c3421bf0fa5','1edb96af-854d-47b8-8683-9a0f7429af4a','war_chariot','land',100,1,'positioned',5,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('30258f82-9283-4b8e-af57-7c3421bf0fa5','1edb96af-854d-47b8-8683-9a0f7429af4a','merchantman','naval',1,10,'Cargo 0','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('30258f82-9283-4b8e-af57-7c3421bf0fa5','1edb96af-854d-47b8-8683-9a0f7429af4a','merchantman','naval',1,10,'Cargo 1','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('30258f82-9283-4b8e-af57-7c3421bf0fa5','1edb96af-854d-47b8-8683-9a0f7429af4a','merchantman','naval',1,10,'Cargo 2','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('30258f82-9283-4b8e-af57-7c3421bf0fa5','1edb96af-854d-47b8-8683-9a0f7429af4a','merchantman','naval',1,10,'Cargo 3','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('30258f82-9283-4b8e-af57-7c3421bf0fa5','1edb96af-854d-47b8-8683-9a0f7429af4a','merchantman','naval',1,10,'Cargo 4','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('30258f82-9283-4b8e-af57-7c3421bf0fa5','1edb96af-854d-47b8-8683-9a0f7429af4a','merchantman','naval',1,10,'Cargo 5','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('30258f82-9283-4b8e-af57-7c3421bf0fa5','1edb96af-854d-47b8-8683-9a0f7429af4a','merchantman','naval',1,10,'Cargo 6','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('30258f82-9283-4b8e-af57-7c3421bf0fa5','1edb96af-854d-47b8-8683-9a0f7429af4a','merchantman','naval',1,10,'Cargo 7','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('30258f82-9283-4b8e-af57-7c3421bf0fa5','1edb96af-854d-47b8-8683-9a0f7429af4a','merchantman','naval',1,10,'Cargo 8','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('30258f82-9283-4b8e-af57-7c3421bf0fa5','1edb96af-854d-47b8-8683-9a0f7429af4a','merchantman','naval',1,10,'Cargo 9','positioned',4,25);

UPDATE provinces SET coastal=true WHERE id='7d7c03dc-de48-4cb3-8d6d-8c4905b8cd2f'; UPDATE settlements SET population=4000 WHERE id='104dc873-d61b-4c40-bbf3-0c8a1b9e1341'; INSERT INTO buildings(settlement_id,building_type,level) VALUES('104dc873-d61b-4c40-bbf3-0c8a1b9e1341','harbour',1) ON CONFLICT DO NOTHING; UPDATE units SET settlement_id='104dc873-d61b-4c40-bbf3-0c8a1b9e1341',support_settlement_id='104dc873-d61b-4c40-bbf3-0c8a1b9e1341',status='garrison',q=NULL,r=NULL WHERE world_id='30258f82-9283-4b8e-af57-7c3421bf0fa5' AND owner_id='1edb96af-854d-47b8-8683-9a0f7429af4a' AND type<>'nomadic_host' AND status<>'disbanded';

INSERT INTO provinces(world_id,map_q,map_r,terrain_type,coastal) VALUES('30258f82-9283-4b8e-af57-7c3421bf0fa5',10,25,'plains',true) RETURNING id;

UPDATE provinces SET terrain_type='plains',coastal=true WHERE id='7b19ef59-4938-403b-bc72-31c9a87f1c79';

INSERT INTO settlements(world_id,province_id,name,culture_id,owner_id,control_type,state,population) VALUES('30258f82-9283-4b8e-af57-7c3421bf0fa5','7b19ef59-4938-403b-bc72-31c9a87f1c79','Petras','minoan','1edb96af-854d-47b8-8683-9a0f7429af4a','colony','active',2000) RETURNING id;

INSERT INTO buildings(settlement_id,building_type,level) VALUES('32249ed4-6ea0-45c6-858a-ef74a54fbeb5','harbour',1);

INSERT INTO provinces(world_id,map_q,map_r,terrain_type,coastal) VALUES('30258f82-9283-4b8e-af57-7c3421bf0fa5',18,25,'plains',true) RETURNING id;

UPDATE provinces SET terrain_type='plains',coastal=true WHERE id='0d768222-ed70-473e-86ca-111f7cc21e9a';

INSERT INTO settlements(world_id,province_id,name,culture_id,owner_id,control_type,state,population) VALUES('30258f82-9283-4b8e-af57-7c3421bf0fa5','0d768222-ed70-473e-86ca-111f7cc21e9a','Amnissos','minoan','1edb96af-854d-47b8-8683-9a0f7429af4a','colony','active',2000) RETURNING id;

INSERT INTO buildings(settlement_id,building_type,level) VALUES('16c3e1af-4887-4e98-a7b2-d016bd1bf371','harbour',1);

INSERT INTO provinces(world_id,map_q,map_r,terrain_type,coastal) VALUES('30258f82-9283-4b8e-af57-7c3421bf0fa5',7,28,'plains',false) RETURNING id;

UPDATE provinces SET terrain_type='plains',coastal=false WHERE id='62bf40ac-7f96-4497-85bc-f6e29a088b76';

INSERT INTO settlements(world_id,province_id,name,culture_id,owner_id,control_type,state,population) VALUES('30258f82-9283-4b8e-af57-7c3421bf0fa5','62bf40ac-7f96-4497-85bc-f6e29a088b76','Inland','minoan','1edb96af-854d-47b8-8683-9a0f7429af4a','colony','active',2000) RETURNING id;

INSERT INTO provinces(world_id,map_q,map_r,terrain_type,coastal) VALUES('30258f82-9283-4b8e-af57-7c3421bf0fa5',10,28,'plains',true) RETURNING id;

UPDATE provinces SET terrain_type='plains',coastal=true WHERE id='b33274c2-126f-4c04-a12d-dfc460770944';

INSERT INTO settlements(world_id,province_id,name,culture_id,owner_id,control_type,state,population) VALUES('30258f82-9283-4b8e-af57-7c3421bf0fa5','b33274c2-126f-4c04-a12d-dfc460770944','Kyme','minoan','1edb96af-854d-47b8-8683-9a0f7429af4a','colony','active',2000) RETURNING id;

INSERT INTO buildings(settlement_id,building_type,level) VALUES('50375157-c687-4482-8f42-a0f9c358950b','harbour',1);

INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) SELECT '104dc873-d61b-4c40-bbf3-0c8a1b9e1341',key,10000,0,1000000,current_world_tick() FROM goods ON CONFLICT(settlement_id,good_key) DO UPDATE SET amount=10000,rate=0,cap=1000000,calc_tick=current_world_tick();

INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) SELECT '32249ed4-6ea0-45c6-858a-ef74a54fbeb5',key,10000,0,1000000,current_world_tick() FROM goods ON CONFLICT(settlement_id,good_key) DO UPDATE SET amount=10000,rate=0,cap=1000000,calc_tick=current_world_tick();

INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) SELECT '16c3e1af-4887-4e98-a7b2-d016bd1bf371',key,10000,0,1000000,current_world_tick() FROM goods ON CONFLICT(settlement_id,good_key) DO UPDATE SET amount=10000,rate=0,cap=1000000,calc_tick=current_world_tick();

INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) SELECT 'cb19ebcb-3a32-4f84-9ed6-73e0272d2556',key,10000,0,1000000,current_world_tick() FROM goods ON CONFLICT(settlement_id,good_key) DO UPDATE SET amount=10000,rate=0,cap=1000000,calc_tick=current_world_tick();

INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) SELECT '50375157-c687-4482-8f42-a0f9c358950b',key,10000,0,1000000,current_world_tick() FROM goods ON CONFLICT(settlement_id,good_key) DO UPDATE SET amount=10000,rate=0,cap=1000000,calc_tick=current_world_tick();

UPDATE settlements SET owner_id='600a2795-4996-4406-9915-a92264c99ef1' WHERE id='50375157-c687-4482-8f42-a0f9c358950b'; INSERT INTO player_scouted_tiles(world_id,player_id,q,r) VALUES('30258f82-9283-4b8e-af57-7c3421bf0fa5','1edb96af-854d-47b8-8683-9a0f7429af4a',10,28) ON CONFLICT DO NOTHING;
