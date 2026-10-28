UPDATE map_tiles SET terrain='coastal_sea',coastal=false WHERE world_id='2f779084-239c-4535-9d26-5b4a02375685'; UPDATE map_tiles SET terrain='plains' WHERE world_id='2f779084-239c-4535-9d26-5b4a02375685' AND q BETWEEN 5 AND 11 AND r BETWEEN 25 AND 28; UPDATE map_tiles SET terrain='plains',coastal=true WHERE world_id='2f779084-239c-4535-9d26-5b4a02375685' AND q=18 AND r=25; UPDATE map_tiles SET coastal=true WHERE world_id='2f779084-239c-4535-9d26-5b4a02375685' AND q=5 AND r=25; UPDATE units SET q=5,r=25,status='positioned' WHERE id='ee29d0c7-f80d-4c42-a1d3-616426dce5e6'; DELETE FROM sea_storms WHERE world_id='2f779084-239c-4535-9d26-5b4a02375685';

INSERT INTO units(world_id,owner_id,type,category,size,ordinal,status,q,r) VALUES('2f779084-239c-4535-9d26-5b4a02375685','50ad5e75-3dbe-4870-8a02-7d91194482e7','elite_infantry','land',100,1,'positioned',5,25);

INSERT INTO units(world_id,owner_id,type,category,size,ordinal,status,q,r) VALUES('2f779084-239c-4535-9d26-5b4a02375685','50ad5e75-3dbe-4870-8a02-7d91194482e7','war_chariot','land',100,1,'positioned',5,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('2f779084-239c-4535-9d26-5b4a02375685','50ad5e75-3dbe-4870-8a02-7d91194482e7','merchantman','naval',1,10,'Cargo 0','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('2f779084-239c-4535-9d26-5b4a02375685','50ad5e75-3dbe-4870-8a02-7d91194482e7','merchantman','naval',1,10,'Cargo 1','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('2f779084-239c-4535-9d26-5b4a02375685','50ad5e75-3dbe-4870-8a02-7d91194482e7','merchantman','naval',1,10,'Cargo 2','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('2f779084-239c-4535-9d26-5b4a02375685','50ad5e75-3dbe-4870-8a02-7d91194482e7','merchantman','naval',1,10,'Cargo 3','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('2f779084-239c-4535-9d26-5b4a02375685','50ad5e75-3dbe-4870-8a02-7d91194482e7','merchantman','naval',1,10,'Cargo 4','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('2f779084-239c-4535-9d26-5b4a02375685','50ad5e75-3dbe-4870-8a02-7d91194482e7','merchantman','naval',1,10,'Cargo 5','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('2f779084-239c-4535-9d26-5b4a02375685','50ad5e75-3dbe-4870-8a02-7d91194482e7','merchantman','naval',1,10,'Cargo 6','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('2f779084-239c-4535-9d26-5b4a02375685','50ad5e75-3dbe-4870-8a02-7d91194482e7','merchantman','naval',1,10,'Cargo 7','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('2f779084-239c-4535-9d26-5b4a02375685','50ad5e75-3dbe-4870-8a02-7d91194482e7','merchantman','naval',1,10,'Cargo 8','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('2f779084-239c-4535-9d26-5b4a02375685','50ad5e75-3dbe-4870-8a02-7d91194482e7','merchantman','naval',1,10,'Cargo 9','positioned',4,25);

UPDATE provinces SET coastal=true WHERE id='103d962b-bba5-410c-8646-81a3f1e795ea'; UPDATE settlements SET population=4000 WHERE id='263a16e1-d2bf-4521-b87e-9d0d68e25dc3'; INSERT INTO buildings(settlement_id,building_type,level) VALUES('263a16e1-d2bf-4521-b87e-9d0d68e25dc3','harbour',1) ON CONFLICT DO NOTHING; UPDATE units SET settlement_id='263a16e1-d2bf-4521-b87e-9d0d68e25dc3',support_settlement_id='263a16e1-d2bf-4521-b87e-9d0d68e25dc3',status='garrison',q=NULL,r=NULL WHERE world_id='2f779084-239c-4535-9d26-5b4a02375685' AND owner_id='50ad5e75-3dbe-4870-8a02-7d91194482e7' AND type<>'nomadic_host' AND status<>'disbanded';

INSERT INTO provinces(world_id,map_q,map_r,terrain_type,coastal) VALUES('2f779084-239c-4535-9d26-5b4a02375685',10,25,'plains',true) RETURNING id;

UPDATE provinces SET terrain_type='plains',coastal=true WHERE id='2e89c830-aa79-4624-b38a-7f546b8deead';

INSERT INTO settlements(world_id,province_id,name,culture_id,owner_id,control_type,state,population) VALUES('2f779084-239c-4535-9d26-5b4a02375685','2e89c830-aa79-4624-b38a-7f546b8deead','Petras','minoan','50ad5e75-3dbe-4870-8a02-7d91194482e7','colony','active',2000) RETURNING id;

INSERT INTO buildings(settlement_id,building_type,level) VALUES('2543dd1d-91f2-4e13-9d29-f7be824f9b14','harbour',1);

INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) SELECT '263a16e1-d2bf-4521-b87e-9d0d68e25dc3',key,10000,0,1000000,current_world_tick() FROM goods ON CONFLICT(settlement_id,good_key) DO UPDATE SET amount=10000,rate=0,cap=1000000,calc_tick=current_world_tick();

INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) SELECT '2543dd1d-91f2-4e13-9d29-f7be824f9b14',key,10000,0,1000000,current_world_tick() FROM goods ON CONFLICT(settlement_id,good_key) DO UPDATE SET amount=10000,rate=0,cap=1000000,calc_tick=current_world_tick();

UPDATE units SET settlement_id='2543dd1d-91f2-4e13-9d29-f7be824f9b14',support_settlement_id='263a16e1-d2bf-4521-b87e-9d0d68e25dc3',q=NULL,r=NULL WHERE world_id='2f779084-239c-4535-9d26-5b4a02375685' AND category='naval'; UPDATE settlement_goods SET amount=0,rate=0 WHERE settlement_id='2543dd1d-91f2-4e13-9d29-f7be824f9b14' AND good_key IN('grain','fish','oil','wine');

UPDATE settlement_goods SET amount=1000 WHERE settlement_id='2543dd1d-91f2-4e13-9d29-f7be824f9b14' AND good_key='fish'
