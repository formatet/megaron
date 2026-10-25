UPDATE map_tiles SET terrain='plains' WHERE world_id='a4c68564-ef58-4be9-90d5-6247d92482a8'; UPDATE units SET q=10,r=27,status='positioned' WHERE id='6ed40deb-d13c-4d68-ae06-af7f0f855b42';

UPDATE settlement_goods SET amount=10000,rate=0,cap=1000000 WHERE settlement_id='60eeb2a3-7199-48dc-8f2e-6fff13e0a198'; DELETE FROM sea_storms WHERE world_id='a4c68564-ef58-4be9-90d5-6247d92482a8';

INSERT INTO provinces(world_id,map_q,map_r,terrain_type) VALUES('a4c68564-ef58-4be9-90d5-6247d92482a8',13,27,'plains') RETURNING id;

INSERT INTO settlements(world_id,province_id,name,culture_id,owner_id,control_type,is_capital,state,population) VALUES('a4c68564-ef58-4be9-90d5-6247d92482a8','a9c183a6-f53d-4d06-a6f8-8a0d9aab1388','Petras','achaean','2bc78f61-5e17-4fdb-804e-6724835870aa','colony',false,'active',500) RETURNING id;

INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) VALUES('0a3d90f4-6978-4974-af3f-c69f7c3e18aa','stone',0,0,10000,0) ON CONFLICT(settlement_id,good_key) DO UPDATE SET amount=0,rate=0,cap=10000;

UPDATE worlds SET current_tick=103,last_tick_at=now()-interval '4.4 seconds' WHERE id='a4c68564-ef58-4be9-90d5-6247d92482a8'; UPDATE settlement_goods SET calc_tick=103 WHERE settlement_id IN ('60eeb2a3-7199-48dc-8f2e-6fff13e0a198','0a3d90f4-6978-4974-af3f-c69f7c3e18aa');

INSERT INTO units(world_id,owner_id,type,category,size,crew,status,q,r,target_q,target_r,march_route,depart_tick,arrive_tick) VALUES('a4c68564-ef58-4be9-90d5-6247d92482a8','2bc78f61-5e17-4fdb-804e-6724835870aa','infantry','land',100,0,'marching',11,29,11,26,'{"version": 2, "end_tick": 108, "start_tick": 103, "departure_at": "2026-10-25T15:48:11.143607+01:00", "offset_milli": 735, "tick_nanoseconds": 6000000000, "hexes": [[11, 29], [11, 28], [11, 27], [11, 26]], "costs": [1000, 1000, 1000]}',103,108);

INSERT INTO messengers(world_id,sender_id,origin_id,destination_id,kind,message_text,status,hex_q,hex_r,sent_at,return_departs_at,arrives_at,land_route) SELECT world_id,owner_id,origin_id,dest_id,'message','Crossing return fixture','returning',13,25,departs_at,departs_at,arrives_at,'{"version": 2, "end_tick": 108, "start_tick": 103, "departure_at": "2026-10-25T15:48:11.143607+01:00", "offset_milli": 735, "tick_nanoseconds": 6000000000, "hexes": [[13, 25], [12, 26], [11, 27], [10, 28]], "costs": [1000, 1000, 1000]}' FROM transports WHERE id='04257b25-85fa-42da-9a13-c1aafad0c87f';
