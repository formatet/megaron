UPDATE map_tiles SET terrain='plains' WHERE world_id='a16e1cd9-5264-4f0c-bf1d-e6539e886110'; UPDATE units SET q=1,r=32,status='positioned' WHERE id='7e26c6f1-308a-4657-b74a-a15abb2eb8a4';

UPDATE settlement_goods SET amount=10000,rate=0,cap=1000000 WHERE settlement_id='9f5499ad-6d8a-4384-a4f8-88ec6d3065e6'; DELETE FROM sea_storms WHERE world_id='a16e1cd9-5264-4f0c-bf1d-e6539e886110';

INSERT INTO provinces(world_id,map_q,map_r,terrain_type) VALUES('a16e1cd9-5264-4f0c-bf1d-e6539e886110',4,32,'plains') RETURNING id;

INSERT INTO settlements(world_id,province_id,name,culture_id,owner_id,control_type,is_capital,state,population) VALUES('a16e1cd9-5264-4f0c-bf1d-e6539e886110','6e4240e5-0917-4f9e-bc7e-ccf7d3569775','Petras','achaean','7a8ccf47-985b-4586-b3ea-4c280c352639','colony',false,'active',500) RETURNING id;

INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) VALUES('bc0b092d-989b-47c9-866f-b8f2084e8cff','stone',0,0,10000,0) ON CONFLICT(settlement_id,good_key) DO UPDATE SET amount=0,rate=0,cap=10000;

UPDATE worlds SET current_tick=103,last_tick_at=now()-interval '4.4 seconds' WHERE id='a16e1cd9-5264-4f0c-bf1d-e6539e886110'; UPDATE settlement_goods SET calc_tick=103 WHERE settlement_id IN ('9f5499ad-6d8a-4384-a4f8-88ec6d3065e6','bc0b092d-989b-47c9-866f-b8f2084e8cff');
