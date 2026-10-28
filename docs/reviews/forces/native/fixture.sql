UPDATE map_tiles SET terrain='coastal_sea',coastal=false WHERE world_id='1f07bbe7-7122-4478-8406-df4ec8829465'; UPDATE map_tiles SET terrain='plains' WHERE world_id='1f07bbe7-7122-4478-8406-df4ec8829465' AND q BETWEEN 5 AND 11 AND r BETWEEN 22 AND 28; UPDATE map_tiles SET terrain='plains',coastal=true WHERE world_id='1f07bbe7-7122-4478-8406-df4ec8829465' AND q=18 AND r=25; UPDATE map_tiles SET coastal=true WHERE world_id='1f07bbe7-7122-4478-8406-df4ec8829465' AND q=5 AND r=25; UPDATE units SET q=5,r=25,status='positioned' WHERE id='eaab0036-1987-40a5-8d24-f2b9a0bae710'; DELETE FROM sea_storms WHERE world_id='1f07bbe7-7122-4478-8406-df4ec8829465';

INSERT INTO units(world_id,owner_id,type,category,size,ordinal,status,q,r) VALUES('1f07bbe7-7122-4478-8406-df4ec8829465','651f567c-2982-4240-b730-6989b4196144','elite_infantry','land',100,1,'positioned',5,25);

INSERT INTO units(world_id,owner_id,type,category,size,ordinal,status,q,r) VALUES('1f07bbe7-7122-4478-8406-df4ec8829465','651f567c-2982-4240-b730-6989b4196144','war_chariot','land',100,1,'positioned',5,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('1f07bbe7-7122-4478-8406-df4ec8829465','651f567c-2982-4240-b730-6989b4196144','galley','naval',1,10,'Dolphin','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('1f07bbe7-7122-4478-8406-df4ec8829465','651f567c-2982-4240-b730-6989b4196144','merchantman','naval',1,10,'Blue Dolphin','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('1f07bbe7-7122-4478-8406-df4ec8829465','651f567c-2982-4240-b730-6989b4196144','war_galley','naval',1,10,'Red Dolphin','positioned',4,25);

UPDATE provinces SET coastal=true WHERE id='d376e74b-b92e-40c6-9c15-f82f29066691'; UPDATE settlements SET population=4000 WHERE id='dcaca3ba-f87d-4d6c-ace7-6def1c99329e'; INSERT INTO buildings(settlement_id,building_type,level) VALUES('dcaca3ba-f87d-4d6c-ace7-6def1c99329e','harbour',1) ON CONFLICT DO NOTHING; UPDATE units SET settlement_id='dcaca3ba-f87d-4d6c-ace7-6def1c99329e',support_settlement_id='dcaca3ba-f87d-4d6c-ace7-6def1c99329e',status='garrison',q=NULL,r=NULL WHERE world_id='1f07bbe7-7122-4478-8406-df4ec8829465' AND owner_id='651f567c-2982-4240-b730-6989b4196144' AND type<>'nomadic_host' AND status<>'disbanded';

INSERT INTO provinces(world_id,map_q,map_r,terrain_type,coastal) VALUES('1f07bbe7-7122-4478-8406-df4ec8829465',10,25,'plains',false) RETURNING id;

UPDATE provinces SET terrain_type='plains',coastal=false WHERE id='58768623-f88f-41dd-b2e0-5007dfbd1ac9';

INSERT INTO settlements(world_id,province_id,name,culture_id,owner_id,control_type,state,population) VALUES('1f07bbe7-7122-4478-8406-df4ec8829465','58768623-f88f-41dd-b2e0-5007dfbd1ac9','Petras','minoan','651f567c-2982-4240-b730-6989b4196144','colony','active',2000) RETURNING id;

INSERT INTO provinces(world_id,map_q,map_r,terrain_type,coastal) VALUES('1f07bbe7-7122-4478-8406-df4ec8829465',18,25,'plains',true) RETURNING id;

UPDATE provinces SET terrain_type='plains',coastal=true WHERE id='301d5cfa-a116-401c-886b-5aecdf05aa39';

INSERT INTO settlements(world_id,province_id,name,culture_id,owner_id,control_type,state,population) VALUES('1f07bbe7-7122-4478-8406-df4ec8829465','301d5cfa-a116-401c-886b-5aecdf05aa39','Amnissos','minoan','651f567c-2982-4240-b730-6989b4196144','colony','active',2000) RETURNING id;

INSERT INTO buildings(settlement_id,building_type,level) VALUES('42e4d013-57d4-4012-b73c-a27253358dc4','harbour',1);

INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) SELECT 'dcaca3ba-f87d-4d6c-ace7-6def1c99329e',key,10000,0,1000000,current_world_tick() FROM goods ON CONFLICT(settlement_id,good_key) DO UPDATE SET amount=10000,rate=0,cap=1000000,calc_tick=current_world_tick();

INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) SELECT '061f248e-42ee-4673-9246-246645b1bf0d',key,10000,0,1000000,current_world_tick() FROM goods ON CONFLICT(settlement_id,good_key) DO UPDATE SET amount=10000,rate=0,cap=1000000,calc_tick=current_world_tick();

INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) SELECT '42e4d013-57d4-4012-b73c-a27253358dc4',key,10000,0,1000000,current_world_tick() FROM goods ON CONFLICT(settlement_id,good_key) DO UPDATE SET amount=10000,rate=0,cap=1000000,calc_tick=current_world_tick();

UPDATE transports SET standing_order_id='bce74e15-83f9-455a-9e64-e46ae9502b9f' WHERE id='31cf35ec-d179-47e2-b8dd-7e6ccfe61322'; UPDATE standing_orders SET ship_unit_id=(SELECT ship_unit_id FROM transports WHERE id='31cf35ec-d179-47e2-b8dd-7e6ccfe61322') WHERE id='bce74e15-83f9-455a-9e64-e46ae9502b9f';

UPDATE transports SET standing_order_id='627752e8-f4da-4cbd-bc52-352fb32ef651' WHERE id='1657f53b-3b89-4fad-b9be-3709bcc6ad55'; UPDATE standing_orders SET ship_unit_id=(SELECT ship_unit_id FROM transports WHERE id='1657f53b-3b89-4fad-b9be-3709bcc6ad55') WHERE id='627752e8-f4da-4cbd-bc52-352fb32ef651';
