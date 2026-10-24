UPDATE players SET wanax_name='Leandros' WHERE id='85bb1abc-5afd-4a72-997c-a60e983e47bb'

UPDATE players SET wanax_name='Myrine' WHERE id='0c1cf68a-0aff-45b1-8c80-650acf321430'

UPDATE players SET wanax_name='Dorian' WHERE id='8ecd1d0b-4e2c-4ed0-a4f2-d966c7376e06'

UPDATE map_tiles SET terrain='plains',coastal=true WHERE world_id='71e3602d-344c-4dcf-9c8c-c69e6291ae38' AND r=42 AND q BETWEEN 1 AND 7; UPDATE map_tiles SET terrain='coastal_sea' WHERE world_id='71e3602d-344c-4dcf-9c8c-c69e6291ae38' AND (q,r) IN((0,43),(1,43),(0,44));

UPDATE units SET q=1,r=42,status='positioned' WHERE id='f2f5376b-4fa3-4224-86f0-f6ecfacc2057';

UPDATE units SET q=7,r=42,status='positioned' WHERE id='c523976e-0017-4426-bf11-f2c7acd46855';

UPDATE units SET q=3,r=42,status='positioned' WHERE id='555c3aa0-2886-4f13-8a69-67fbe9df97c7'; UPDATE founder_phase SET is_refugee=true WHERE host_unit_id='555c3aa0-2886-4f13-8a69-67fbe9df97c7'; UPDATE worlds SET current_tick=100,last_tick_at=now() WHERE id='71e3602d-344c-4dcf-9c8c-c69e6291ae38';

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('71e3602d-344c-4dcf-9c8c-c69e6291ae38','85bb1abc-5afd-4a72-997c-a60e983e47bb','d0ab7732-d2f7-486a-a66a-ab59626d1d6b') ON CONFLICT DO NOTHING

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('71e3602d-344c-4dcf-9c8c-c69e6291ae38','0c1cf68a-0aff-45b1-8c80-650acf321430','71bb4d81-9a24-47aa-b793-5f9889ca37e4') ON CONFLICT DO NOTHING

UPDATE provinces SET coastal=true WHERE id='71bb4d81-9a24-47aa-b793-5f9889ca37e4'; INSERT INTO buildings(settlement_id,building_type,level,god_id) VALUES('75d84511-36d6-441e-a451-f1fb41d3153c','harbour',1,NULL),('75d84511-36d6-441e-a451-f1fb41d3153c','temple',3,'potnia') ON CONFLICT DO NOTHING; UPDATE player_world_records SET kharis_amount=90,kharis_rate=0,kharis_calc_tick=100 WHERE world_id='71e3602d-344c-4dcf-9c8c-c69e6291ae38' AND player_id='85bb1abc-5afd-4a72-997c-a60e983e47bb'; UPDATE settlement_goods SET amount=10000,rate=0,cap=1000000,calc_tick=100 WHERE settlement_id IN('75d84511-36d6-441e-a451-f1fb41d3153c','f9bbdc60-f3a9-4886-b82e-dbcf8730501d'); DELETE FROM settlement_placement WHERE settlement_id='75d84511-36d6-441e-a451-f1fb41d3153c'; INSERT INTO settlement_placement(settlement_id,gubbe_ordinal,target_kind,hex_q,hex_r,good_key) VALUES('75d84511-36d6-441e-a451-f1fb41d3153c',1,'hex',0,43,'fish');

INSERT INTO sea_storms(world_id,heading,created_tick) VALUES('71e3602d-344c-4dcf-9c8c-c69e6291ae38',0,100) RETURNING id

INSERT INTO sea_storm_track(storm_id,tick,slot,q,r) VALUES('58bbe8af-e403-42b0-bd87-1ec20b377bdc',100,0,0,43),('58bbe8af-e403-42b0-bd87-1ec20b377bdc',100,1,1,43),('58bbe8af-e403-42b0-bd87-1ec20b377bdc',100,2,0,44);

UPDATE scheduled_events SET due_tick=100,process_after=now() WHERE world_id='71e3602d-344c-4dcf-9c8c-c69e6291ae38' AND event_type='MessengerArrival' AND payload->>'messenger_id'='ec9da498-90bb-4599-a50c-d14aa01d7e16';

UPDATE map_tiles SET copper_deposit=false,tin_deposit=false,silver_deposit=false,incense_deposit=false WHERE world_id='71e3602d-344c-4dcf-9c8c-c69e6291ae38'; UPDATE map_tiles SET terrain='hills',silver_deposit=true WHERE world_id='71e3602d-344c-4dcf-9c8c-c69e6291ae38' AND q=2 AND r=60;

UPDATE settlement_goods SET amount=CASE WHEN good_key='grain' THEN 29 WHEN good_key IN('fish','oil','wine','livestock') THEN 0 ELSE 10000 END,rate=0,calc_tick=100 WHERE settlement_id='75d84511-36d6-441e-a451-f1fb41d3153c';

DELETE FROM prayer_cooldowns WHERE world_id='71e3602d-344c-4dcf-9c8c-c69e6291ae38' AND player_id='85bb1abc-5afd-4a72-997c-a60e983e47bb'; DELETE FROM player_scouted_tiles WHERE world_id='71e3602d-344c-4dcf-9c8c-c69e6291ae38' AND player_id='85bb1abc-5afd-4a72-997c-a60e983e47bb' AND (abs(q-2)+abs(q+r-2-60)+abs(r-60))/2<=2;

UPDATE settlement_goods SET amount=1000,rate=0,calc_tick=100 WHERE settlement_id='75d84511-36d6-441e-a451-f1fb41d3153c' AND good_key IN('grain','silver');

UPDATE settlement_goods SET amount=CASE WHEN good_key='grain' THEN 29 WHEN good_key IN('fish','oil','wine','livestock') THEN 0 ELSE 10000 END,rate=0,calc_tick=100 WHERE settlement_id='75d84511-36d6-441e-a451-f1fb41d3153c';

DELETE FROM prayer_cooldowns WHERE world_id='71e3602d-344c-4dcf-9c8c-c69e6291ae38' AND player_id='85bb1abc-5afd-4a72-997c-a60e983e47bb'; DELETE FROM player_scouted_tiles WHERE world_id='71e3602d-344c-4dcf-9c8c-c69e6291ae38' AND player_id='85bb1abc-5afd-4a72-997c-a60e983e47bb' AND (abs(q-2)+abs(q+r-2-60)+abs(r-60))/2<=2;

UPDATE settlement_goods SET amount=1000,rate=0,calc_tick=100 WHERE settlement_id='75d84511-36d6-441e-a451-f1fb41d3153c' AND good_key IN('grain','silver');
