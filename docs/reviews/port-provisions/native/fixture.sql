UPDATE map_tiles SET terrain='coastal_sea',coastal=false WHERE world_id='ef9e0142-daf6-4967-a9eb-a0c38634165b'; UPDATE map_tiles SET terrain='plains' WHERE world_id='ef9e0142-daf6-4967-a9eb-a0c38634165b' AND q BETWEEN 5 AND 11 AND r BETWEEN 25 AND 28; UPDATE map_tiles SET terrain='plains',coastal=true WHERE world_id='ef9e0142-daf6-4967-a9eb-a0c38634165b' AND q=18 AND r=25; UPDATE map_tiles SET coastal=true WHERE world_id='ef9e0142-daf6-4967-a9eb-a0c38634165b' AND q=5 AND r=25; UPDATE units SET q=5,r=25,status='positioned' WHERE id='433f43a4-3a04-4d1d-a085-9f7f52161617'; DELETE FROM sea_storms WHERE world_id='ef9e0142-daf6-4967-a9eb-a0c38634165b';

INSERT INTO units(world_id,owner_id,type,category,size,ordinal,status,q,r) VALUES('ef9e0142-daf6-4967-a9eb-a0c38634165b','d5e69df2-2ec1-4721-b94a-71482abbeee6','elite_infantry','land',100,1,'positioned',5,25);

INSERT INTO units(world_id,owner_id,type,category,size,ordinal,status,q,r) VALUES('ef9e0142-daf6-4967-a9eb-a0c38634165b','d5e69df2-2ec1-4721-b94a-71482abbeee6','war_chariot','land',100,1,'positioned',5,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('ef9e0142-daf6-4967-a9eb-a0c38634165b','d5e69df2-2ec1-4721-b94a-71482abbeee6','merchantman','naval',1,10,'Cargo 0','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('ef9e0142-daf6-4967-a9eb-a0c38634165b','d5e69df2-2ec1-4721-b94a-71482abbeee6','merchantman','naval',1,10,'Cargo 1','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('ef9e0142-daf6-4967-a9eb-a0c38634165b','d5e69df2-2ec1-4721-b94a-71482abbeee6','merchantman','naval',1,10,'Cargo 2','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('ef9e0142-daf6-4967-a9eb-a0c38634165b','d5e69df2-2ec1-4721-b94a-71482abbeee6','merchantman','naval',1,10,'Cargo 3','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('ef9e0142-daf6-4967-a9eb-a0c38634165b','d5e69df2-2ec1-4721-b94a-71482abbeee6','merchantman','naval',1,10,'Cargo 4','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('ef9e0142-daf6-4967-a9eb-a0c38634165b','d5e69df2-2ec1-4721-b94a-71482abbeee6','merchantman','naval',1,10,'Cargo 5','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('ef9e0142-daf6-4967-a9eb-a0c38634165b','d5e69df2-2ec1-4721-b94a-71482abbeee6','merchantman','naval',1,10,'Cargo 6','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('ef9e0142-daf6-4967-a9eb-a0c38634165b','d5e69df2-2ec1-4721-b94a-71482abbeee6','merchantman','naval',1,10,'Cargo 7','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('ef9e0142-daf6-4967-a9eb-a0c38634165b','d5e69df2-2ec1-4721-b94a-71482abbeee6','merchantman','naval',1,10,'Cargo 8','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('ef9e0142-daf6-4967-a9eb-a0c38634165b','d5e69df2-2ec1-4721-b94a-71482abbeee6','merchantman','naval',1,10,'Cargo 9','positioned',4,25);

UPDATE provinces SET coastal=true WHERE id='e5b43be3-6ce4-4d50-8bfc-05043f4a1cf5'; UPDATE settlements SET population=4000 WHERE id='562f7017-08b3-4414-9a43-3cee6ebac93b'; INSERT INTO buildings(settlement_id,building_type,level) VALUES('562f7017-08b3-4414-9a43-3cee6ebac93b','harbour',1) ON CONFLICT DO NOTHING; UPDATE units SET settlement_id='562f7017-08b3-4414-9a43-3cee6ebac93b',support_settlement_id='562f7017-08b3-4414-9a43-3cee6ebac93b',status='garrison',q=NULL,r=NULL WHERE world_id='ef9e0142-daf6-4967-a9eb-a0c38634165b' AND owner_id='d5e69df2-2ec1-4721-b94a-71482abbeee6' AND type<>'nomadic_host' AND status<>'disbanded';

INSERT INTO provinces(world_id,map_q,map_r,terrain_type,coastal) VALUES('ef9e0142-daf6-4967-a9eb-a0c38634165b',10,25,'plains',true) RETURNING id;

UPDATE provinces SET terrain_type='plains',coastal=true WHERE id='46ae21f3-bd51-43b0-b0b1-b1c74e095896';

INSERT INTO settlements(world_id,province_id,name,culture_id,owner_id,control_type,state,population) VALUES('ef9e0142-daf6-4967-a9eb-a0c38634165b','46ae21f3-bd51-43b0-b0b1-b1c74e095896','Petras','minoan','d5e69df2-2ec1-4721-b94a-71482abbeee6','colony','active',2000) RETURNING id;

INSERT INTO buildings(settlement_id,building_type,level) VALUES('7af7961f-7176-4e47-9320-b07e2b4dd1cf','harbour',1);

INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) SELECT '562f7017-08b3-4414-9a43-3cee6ebac93b',key,10000,0,1000000,current_world_tick() FROM goods ON CONFLICT(settlement_id,good_key) DO UPDATE SET amount=10000,rate=0,cap=1000000,calc_tick=current_world_tick();

INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) SELECT '7af7961f-7176-4e47-9320-b07e2b4dd1cf',key,10000,0,1000000,current_world_tick() FROM goods ON CONFLICT(settlement_id,good_key) DO UPDATE SET amount=10000,rate=0,cap=1000000,calc_tick=current_world_tick();

UPDATE units SET settlement_id='7af7961f-7176-4e47-9320-b07e2b4dd1cf',support_settlement_id='562f7017-08b3-4414-9a43-3cee6ebac93b',q=NULL,r=NULL WHERE world_id='ef9e0142-daf6-4967-a9eb-a0c38634165b' AND category='naval'; UPDATE settlement_goods SET amount=0,rate=0 WHERE settlement_id='7af7961f-7176-4e47-9320-b07e2b4dd1cf' AND good_key IN('grain','fish','oil','wine');

UPDATE settlement_goods SET amount=1000 WHERE settlement_id='7af7961f-7176-4e47-9320-b07e2b4dd1cf' AND good_key='fish'
