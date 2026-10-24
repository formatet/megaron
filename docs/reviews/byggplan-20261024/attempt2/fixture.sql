UPDATE players SET wanax_name='Leandros' WHERE id='b075f66c-cf91-4d3d-bd8b-4846f98fcffc'

UPDATE players SET wanax_name='Myrine' WHERE id='d812040b-d33e-4392-a24e-4e423861f558'

UPDATE players SET wanax_name='Dorian' WHERE id='75c830aa-6e5e-4c82-93a3-8fc9021401c2'

UPDATE map_tiles SET terrain='plains',coastal=true WHERE world_id='98187eb5-4c0c-4d50-9b69-479dcb95b361' AND r=29 AND q BETWEEN 1 AND 7; UPDATE map_tiles SET terrain='coastal_sea' WHERE world_id='98187eb5-4c0c-4d50-9b69-479dcb95b361' AND q=0 AND r=30;

UPDATE units SET q=1,r=29,status='positioned' WHERE id='6da8d656-b0d7-48fd-9dca-dd191205c343';

UPDATE units SET q=7,r=29,status='positioned' WHERE id='24b149a0-09d6-4733-a5f0-2ed90accaa89';

UPDATE units SET q=3,r=29,status='positioned' WHERE id='c7fb5643-051e-4bf1-beed-0091dadd9300'; UPDATE founder_phase SET is_refugee=true WHERE host_unit_id='c7fb5643-051e-4bf1-beed-0091dadd9300'; UPDATE worlds SET current_tick=100,last_tick_at=now() WHERE id='98187eb5-4c0c-4d50-9b69-479dcb95b361';

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('98187eb5-4c0c-4d50-9b69-479dcb95b361','b075f66c-cf91-4d3d-bd8b-4846f98fcffc','07f3604c-1930-4900-be33-5b976dcaa582') ON CONFLICT DO NOTHING

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('98187eb5-4c0c-4d50-9b69-479dcb95b361','d812040b-d33e-4392-a24e-4e423861f558','531733ab-fdd1-483c-b8cc-cc151a570de4') ON CONFLICT DO NOTHING

UPDATE provinces SET coastal=true WHERE id='531733ab-fdd1-483c-b8cc-cc151a570de4'; INSERT INTO buildings(settlement_id,building_type,level,god_id) VALUES('20b12d61-cc8d-4c2c-b9da-591e160e69c2','harbour',1,NULL),('20b12d61-cc8d-4c2c-b9da-591e160e69c2','temple',3,'potnia') ON CONFLICT DO NOTHING; UPDATE player_world_records SET kharis_amount=90,kharis_rate=0,kharis_calc_tick=100 WHERE world_id='98187eb5-4c0c-4d50-9b69-479dcb95b361' AND player_id='b075f66c-cf91-4d3d-bd8b-4846f98fcffc'; UPDATE settlement_goods SET amount=10000,rate=0,cap=1000000,calc_tick=100 WHERE settlement_id IN('20b12d61-cc8d-4c2c-b9da-591e160e69c2','71c054d4-4fc3-4ed8-a1c3-759bbb65cba5'); DELETE FROM settlement_placement WHERE settlement_id='20b12d61-cc8d-4c2c-b9da-591e160e69c2'; INSERT INTO settlement_placement(settlement_id,gubbe_ordinal,target_kind,hex_q,hex_r,good_key) VALUES('20b12d61-cc8d-4c2c-b9da-591e160e69c2',1,'hex',0,30,'fish');

INSERT INTO sea_storms(world_id,heading,created_tick) VALUES('98187eb5-4c0c-4d50-9b69-479dcb95b361',0,100) RETURNING id

INSERT INTO sea_storm_track(storm_id,tick,slot,q,r) VALUES('2cd92591-9d33-4d2f-805f-89b7fbf0833d',100,0,0,30);

UPDATE scheduled_events SET due_tick=100,process_after=now() WHERE world_id='98187eb5-4c0c-4d50-9b69-479dcb95b361' AND event_type='MessengerArrival' AND payload->>'messenger_id'='a18ecdad-e415-401b-bd4e-ec303faceebe';

UPDATE map_tiles SET copper_deposit=false,tin_deposit=false,silver_deposit=false WHERE world_id='98187eb5-4c0c-4d50-9b69-479dcb95b361'; UPDATE map_tiles SET terrain='hills',silver_deposit=true WHERE world_id='98187eb5-4c0c-4d50-9b69-479dcb95b361' AND q=1 AND r=50;

UPDATE settlement_goods SET amount=CASE WHEN good_key='grain' THEN 29 WHEN good_key IN('fish','oil','wine') THEN 0 ELSE 10000 END,rate=0,calc_tick=100 WHERE settlement_id='20b12d61-cc8d-4c2c-b9da-591e160e69c2';

DELETE FROM prayer_cooldowns WHERE world_id='98187eb5-4c0c-4d50-9b69-479dcb95b361' AND player_id='b075f66c-cf91-4d3d-bd8b-4846f98fcffc'; DELETE FROM player_scouted_tiles WHERE world_id='98187eb5-4c0c-4d50-9b69-479dcb95b361' AND player_id='b075f66c-cf91-4d3d-bd8b-4846f98fcffc' AND (abs(q-1)+abs(q+r-1-50)+abs(r-50))/2<=1;
