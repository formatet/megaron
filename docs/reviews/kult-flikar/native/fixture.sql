UPDATE map_tiles SET terrain='plains',coastal=false WHERE world_id='95fe5782-8645-4c79-9755-3853bf6bccb2';

INSERT INTO provinces(world_id,map_q,map_r,terrain_type,coastal) VALUES('95fe5782-8645-4c79-9755-3853bf6bccb2',10,10,'plains',false) RETURNING id;

INSERT INTO settlements(world_id,province_id,name,culture_id,owner_id,control_type,is_capital,state,population) VALUES('95fe5782-8645-4c79-9755-3853bf6bccb2','55a352c1-7f63-488d-b14d-9b4d5b9f8db9','Amnissos','minoan','70e49cd3-605b-4cc2-937b-6649fb6b4f86','colony',false,'active',2000) RETURNING id;

INSERT INTO provinces(world_id,map_q,map_r,terrain_type,coastal) VALUES('95fe5782-8645-4c79-9755-3853bf6bccb2',20,20,'plains',false) RETURNING id;

INSERT INTO settlements(world_id,province_id,name,culture_id,owner_id,control_type,is_capital,state,population) VALUES('95fe5782-8645-4c79-9755-3853bf6bccb2','3dbb7214-32ac-43d7-a789-fe8e90c59755','Kommos','minoan','70e49cd3-605b-4cc2-937b-6649fb6b4f86','colony',false,'active',2000) RETURNING id;

UPDATE settlements SET culture_id='minoan',population=2000 WHERE id='6982b477-6e21-42b1-ad87-5260151dfab8'; INSERT INTO buildings(settlement_id,building_type,level,god_id) VALUES('6982b477-6e21-42b1-ad87-5260151dfab8','temple',1,'sea_daimons_stillness') ON CONFLICT DO NOTHING; INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) SELECT '6982b477-6e21-42b1-ad87-5260151dfab8',key,10000,0,1000000,current_world_tick() FROM goods ON CONFLICT(settlement_id,good_key) DO UPDATE SET amount=10000,rate=0,cap=1000000,calc_tick=current_world_tick();

UPDATE settlements SET culture_id='minoan',population=2000 WHERE id='ba9685ea-3a0b-486d-9514-d7fafd6e84ff'; INSERT INTO buildings(settlement_id,building_type,level,god_id) VALUES('ba9685ea-3a0b-486d-9514-d7fafd6e84ff','temple',1,'asterion') ON CONFLICT DO NOTHING; INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) SELECT 'ba9685ea-3a0b-486d-9514-d7fafd6e84ff',key,10000,0,1000000,current_world_tick() FROM goods ON CONFLICT(settlement_id,good_key) DO UPDATE SET amount=10000,rate=0,cap=1000000,calc_tick=current_world_tick();

UPDATE settlements SET culture_id='minoan',population=2000 WHERE id='245ab88b-814f-4723-9062-b7a74bafb31c'; INSERT INTO buildings(settlement_id,building_type,level,god_id) VALUES('245ab88b-814f-4723-9062-b7a74bafb31c','temple',1,'asterion') ON CONFLICT DO NOTHING; INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) SELECT '245ab88b-814f-4723-9062-b7a74bafb31c',key,10000,0,1000000,current_world_tick() FROM goods ON CONFLICT(settlement_id,good_key) DO UPDATE SET amount=10000,rate=0,cap=1000000,calc_tick=current_world_tick();

UPDATE player_world_records SET kharis_amount=70,kharis_rate=0,kharis_calc_tick=current_world_tick() WHERE player_id='70e49cd3-605b-4cc2-937b-6649fb6b4f86' AND world_id='95fe5782-8645-4c79-9755-3853bf6bccb2';
