UPDATE players SET wanax_name='Leandros' WHERE id='a8cf8a62-6aca-44e0-a33c-d9f1fc01542c'

UPDATE players SET wanax_name='Myrine' WHERE id='4db88077-8ce3-4158-92cd-f4e4abbbdf5a'

UPDATE players SET wanax_name='Dorian' WHERE id='d655eab8-6611-4ae7-a4ca-e219ab5d4149'

UPDATE map_tiles SET terrain='plains',coastal=true WHERE world_id='42a7a78d-1045-4591-b63f-a6d490d93a07' AND r=52 AND q BETWEEN 1 AND 7; UPDATE map_tiles SET terrain='coastal_sea' WHERE world_id='42a7a78d-1045-4591-b63f-a6d490d93a07' AND (q,r) IN((0,53),(1,53),(0,54));

UPDATE units SET q=1,r=52,status='positioned' WHERE id='c743dff0-87d9-4dfc-989e-7714bb0d1451';

UPDATE units SET q=1,r=52,status='garrison',settlement_id='770c45e9-5b74-4cbb-9d84-f3e7b143d8f1',support_settlement_id='770c45e9-5b74-4cbb-9d84-f3e7b143d8f1' WHERE world_id='42a7a78d-1045-4591-b63f-a6d490d93a07' AND owner_id='a8cf8a62-6aca-44e0-a33c-d9f1fc01542c' AND type='spearman';

UPDATE units SET q=7,r=52,status='positioned' WHERE id='0ceb1c68-22fa-4e09-99e3-d02d1fb33fc4';

UPDATE units SET q=7,r=52,status='garrison',settlement_id='4eacbf6c-b52a-4af2-bf09-160af7c5a94c',support_settlement_id='4eacbf6c-b52a-4af2-bf09-160af7c5a94c' WHERE world_id='42a7a78d-1045-4591-b63f-a6d490d93a07' AND owner_id='4db88077-8ce3-4158-92cd-f4e4abbbdf5a' AND type='spearman';

UPDATE units SET q=3,r=52,status='positioned' WHERE id='c9ab3708-c9b8-4c1f-bf7d-8e3cfa476afb'; UPDATE founder_phase SET is_refugee=true WHERE host_unit_id='c9ab3708-c9b8-4c1f-bf7d-8e3cfa476afb'; UPDATE worlds SET current_tick=100,last_tick_at=now() WHERE id='42a7a78d-1045-4591-b63f-a6d490d93a07';

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('42a7a78d-1045-4591-b63f-a6d490d93a07','a8cf8a62-6aca-44e0-a33c-d9f1fc01542c','a8ee9bed-eeb6-4ed9-a72d-32b36e6f7b8e') ON CONFLICT DO NOTHING

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('42a7a78d-1045-4591-b63f-a6d490d93a07','4db88077-8ce3-4158-92cd-f4e4abbbdf5a','ce01d542-58d1-4b3d-9fdd-57037f71b1f2') ON CONFLICT DO NOTHING

UPDATE provinces SET coastal=true WHERE id='ce01d542-58d1-4b3d-9fdd-57037f71b1f2'; INSERT INTO buildings(settlement_id,building_type,level,god_id) VALUES('770c45e9-5b74-4cbb-9d84-f3e7b143d8f1','harbour',1,NULL),('770c45e9-5b74-4cbb-9d84-f3e7b143d8f1','temple',3,'potnia') ON CONFLICT DO NOTHING; UPDATE player_world_records SET kharis_amount=90,kharis_rate=0,kharis_calc_tick=100 WHERE world_id='42a7a78d-1045-4591-b63f-a6d490d93a07' AND player_id='a8cf8a62-6aca-44e0-a33c-d9f1fc01542c'; UPDATE settlement_goods SET amount=10000,rate=0,cap=1000000,calc_tick=100 WHERE settlement_id IN('770c45e9-5b74-4cbb-9d84-f3e7b143d8f1','4eacbf6c-b52a-4af2-bf09-160af7c5a94c'); DELETE FROM settlement_placement WHERE settlement_id='770c45e9-5b74-4cbb-9d84-f3e7b143d8f1'; INSERT INTO settlement_placement(settlement_id,gubbe_ordinal,target_kind,hex_q,hex_r,good_key) VALUES('770c45e9-5b74-4cbb-9d84-f3e7b143d8f1',1,'hex',0,53,'fish');

INSERT INTO sea_storms(world_id,heading,created_tick) VALUES('42a7a78d-1045-4591-b63f-a6d490d93a07',0,100) RETURNING id

INSERT INTO sea_storm_track(storm_id,tick,slot,q,r) VALUES('7c57a932-78c7-48bd-a151-007748d55dda',100,0,0,53),('7c57a932-78c7-48bd-a151-007748d55dda',100,1,1,53),('7c57a932-78c7-48bd-a151-007748d55dda',100,2,0,54);

UPDATE scheduled_events SET due_tick=100,process_after=now() WHERE world_id='42a7a78d-1045-4591-b63f-a6d490d93a07' AND event_type='MessengerArrival' AND payload->>'messenger_id'='41613e08-e87b-46ef-889e-9571a64e13f6';

UPDATE map_tiles SET copper_deposit=false,tin_deposit=false,silver_deposit=false,incense_deposit=false WHERE world_id='42a7a78d-1045-4591-b63f-a6d490d93a07'; UPDATE map_tiles SET terrain='hills',silver_deposit=true WHERE world_id='42a7a78d-1045-4591-b63f-a6d490d93a07' AND q=2 AND r=28;

UPDATE settlement_goods SET amount=CASE WHEN good_key='grain' THEN 29 WHEN good_key IN('fish','oil','wine','livestock') THEN 0 ELSE 10000 END,rate=0,calc_tick=100 WHERE settlement_id='770c45e9-5b74-4cbb-9d84-f3e7b143d8f1';

DELETE FROM prayer_cooldowns WHERE world_id='42a7a78d-1045-4591-b63f-a6d490d93a07' AND player_id='a8cf8a62-6aca-44e0-a33c-d9f1fc01542c'; DELETE FROM player_scouted_tiles WHERE world_id='42a7a78d-1045-4591-b63f-a6d490d93a07' AND player_id='a8cf8a62-6aca-44e0-a33c-d9f1fc01542c' AND (abs(q-2)+abs(q+r-2-28)+abs(r-28))/2<=2;

UPDATE settlement_goods SET amount=1000,rate=0,calc_tick=100 WHERE settlement_id='770c45e9-5b74-4cbb-9d84-f3e7b143d8f1' AND good_key IN('grain','silver');

UPDATE settlement_goods SET amount=CASE WHEN good_key='grain' THEN 29 WHEN good_key IN('fish','oil','wine','livestock') THEN 0 ELSE 10000 END,rate=0,calc_tick=100 WHERE settlement_id='770c45e9-5b74-4cbb-9d84-f3e7b143d8f1';

DELETE FROM prayer_cooldowns WHERE world_id='42a7a78d-1045-4591-b63f-a6d490d93a07' AND player_id='a8cf8a62-6aca-44e0-a33c-d9f1fc01542c'; DELETE FROM player_scouted_tiles WHERE world_id='42a7a78d-1045-4591-b63f-a6d490d93a07' AND player_id='a8cf8a62-6aca-44e0-a33c-d9f1fc01542c' AND (abs(q-2)+abs(q+r-2-28)+abs(r-28))/2<=2;

UPDATE settlement_goods SET amount=1000,rate=0,calc_tick=100 WHERE settlement_id='770c45e9-5b74-4cbb-9d84-f3e7b143d8f1' AND good_key IN('grain','silver');
