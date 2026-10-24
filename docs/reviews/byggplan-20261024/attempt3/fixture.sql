UPDATE players SET wanax_name='Leandros' WHERE id='43d14783-a6b6-416f-a465-d03cb35275cd'

UPDATE players SET wanax_name='Myrine' WHERE id='abba93cd-46fb-49d7-94d3-4ea675971a71'

UPDATE players SET wanax_name='Dorian' WHERE id='f663cebd-8736-4948-987e-372ab2a352f1'

UPDATE map_tiles SET terrain='plains',coastal=true WHERE world_id='3900a931-5827-4e3a-8683-23730d03f59e' AND r=36 AND q BETWEEN 1 AND 7; UPDATE map_tiles SET terrain='coastal_sea' WHERE world_id='3900a931-5827-4e3a-8683-23730d03f59e' AND q=0 AND r=37;

UPDATE units SET q=1,r=36,status='positioned' WHERE id='fe78e33b-adb7-4ec9-80fa-eb07a3cf7811';

UPDATE units SET q=7,r=36,status='positioned' WHERE id='333f4e7e-44e8-412e-aba6-c47bd8bdb8b1';

UPDATE units SET q=3,r=36,status='positioned' WHERE id='1e68cf24-aeaf-48a6-91de-24ede7fb2dd0'; UPDATE founder_phase SET is_refugee=true WHERE host_unit_id='1e68cf24-aeaf-48a6-91de-24ede7fb2dd0'; UPDATE worlds SET current_tick=100,last_tick_at=now() WHERE id='3900a931-5827-4e3a-8683-23730d03f59e';

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('3900a931-5827-4e3a-8683-23730d03f59e','43d14783-a6b6-416f-a465-d03cb35275cd','4c4def97-f986-4d41-b41c-6674aefffc0d') ON CONFLICT DO NOTHING

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('3900a931-5827-4e3a-8683-23730d03f59e','abba93cd-46fb-49d7-94d3-4ea675971a71','b21ac2b9-5c97-49fb-89ce-686aefe67097') ON CONFLICT DO NOTHING

UPDATE provinces SET coastal=true WHERE id='b21ac2b9-5c97-49fb-89ce-686aefe67097'; INSERT INTO buildings(settlement_id,building_type,level,god_id) VALUES('0f9e0bab-12d4-4871-ae07-5e85852790c8','harbour',1,NULL),('0f9e0bab-12d4-4871-ae07-5e85852790c8','temple',3,'potnia') ON CONFLICT DO NOTHING; UPDATE player_world_records SET kharis_amount=90,kharis_rate=0,kharis_calc_tick=100 WHERE world_id='3900a931-5827-4e3a-8683-23730d03f59e' AND player_id='43d14783-a6b6-416f-a465-d03cb35275cd'; UPDATE settlement_goods SET amount=10000,rate=0,cap=1000000,calc_tick=100 WHERE settlement_id IN('0f9e0bab-12d4-4871-ae07-5e85852790c8','2096f0de-0db8-423b-b290-c0522d4c2703'); DELETE FROM settlement_placement WHERE settlement_id='0f9e0bab-12d4-4871-ae07-5e85852790c8'; INSERT INTO settlement_placement(settlement_id,gubbe_ordinal,target_kind,hex_q,hex_r,good_key) VALUES('0f9e0bab-12d4-4871-ae07-5e85852790c8',1,'hex',0,37,'fish');

INSERT INTO sea_storms(world_id,heading,created_tick) VALUES('3900a931-5827-4e3a-8683-23730d03f59e',0,100) RETURNING id

INSERT INTO sea_storm_track(storm_id,tick,slot,q,r) VALUES('ab99e924-deae-4632-81ea-eecf57a02e3b',100,0,0,37);

UPDATE scheduled_events SET due_tick=100,process_after=now() WHERE world_id='3900a931-5827-4e3a-8683-23730d03f59e' AND event_type='MessengerArrival' AND payload->>'messenger_id'='94934e2f-39dd-40d2-b333-fbab3cac370c';

UPDATE map_tiles SET copper_deposit=false,tin_deposit=false,silver_deposit=false WHERE world_id='3900a931-5827-4e3a-8683-23730d03f59e'; UPDATE map_tiles SET terrain='hills',silver_deposit=true WHERE world_id='3900a931-5827-4e3a-8683-23730d03f59e' AND q=1 AND r=65;

UPDATE settlement_goods SET amount=CASE WHEN good_key='grain' THEN 29 WHEN good_key IN('fish','oil','wine') THEN 0 ELSE 10000 END,rate=0,calc_tick=100 WHERE settlement_id='0f9e0bab-12d4-4871-ae07-5e85852790c8';

DELETE FROM prayer_cooldowns WHERE world_id='3900a931-5827-4e3a-8683-23730d03f59e' AND player_id='43d14783-a6b6-416f-a465-d03cb35275cd'; DELETE FROM player_scouted_tiles WHERE world_id='3900a931-5827-4e3a-8683-23730d03f59e' AND player_id='43d14783-a6b6-416f-a465-d03cb35275cd' AND (abs(q-1)+abs(q+r-1-65)+abs(r-65))/2<=1;
