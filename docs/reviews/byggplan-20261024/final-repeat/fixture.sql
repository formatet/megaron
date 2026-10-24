UPDATE players SET wanax_name='Leandros' WHERE id='53812810-a8e6-4ff7-8dcb-db4b8c2b92ef'

UPDATE players SET wanax_name='Myrine' WHERE id='f4788793-b620-424b-b724-7ec56796d43e'

UPDATE players SET wanax_name='Dorian' WHERE id='f9426974-b54b-46d2-b533-54072b07c3f7'

UPDATE map_tiles SET terrain='plains',coastal=true WHERE world_id='719d4066-f194-479f-b501-62277a2db75d' AND r=54 AND q BETWEEN 1 AND 7; UPDATE map_tiles SET terrain='coastal_sea' WHERE world_id='719d4066-f194-479f-b501-62277a2db75d' AND (q,r) IN((0,55),(1,55),(0,56));

UPDATE units SET q=1,r=54,status='positioned' WHERE id='17b800ee-1d68-47a8-970c-f41b9a9879d9';

UPDATE units SET q=1,r=54,status='garrison',settlement_id='79aeacfc-c9ac-48d9-a62b-cc1689e23d9b',support_settlement_id='79aeacfc-c9ac-48d9-a62b-cc1689e23d9b' WHERE world_id='719d4066-f194-479f-b501-62277a2db75d' AND owner_id='53812810-a8e6-4ff7-8dcb-db4b8c2b92ef' AND type='spearman';

UPDATE units SET q=7,r=54,status='positioned' WHERE id='09bb35b8-15f7-402f-9f79-09fe68c60f6e';

UPDATE units SET q=7,r=54,status='garrison',settlement_id='4af82da9-ecba-4dc4-ade0-b5733285bedc',support_settlement_id='4af82da9-ecba-4dc4-ade0-b5733285bedc' WHERE world_id='719d4066-f194-479f-b501-62277a2db75d' AND owner_id='f4788793-b620-424b-b724-7ec56796d43e' AND type='spearman';

UPDATE units SET q=3,r=54,status='positioned' WHERE id='b1fec5e5-5bab-4302-8ebf-45f7c2e1f0c3'; UPDATE founder_phase SET is_refugee=true WHERE host_unit_id='b1fec5e5-5bab-4302-8ebf-45f7c2e1f0c3'; UPDATE worlds SET current_tick=100,last_tick_at=now() WHERE id='719d4066-f194-479f-b501-62277a2db75d';

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('719d4066-f194-479f-b501-62277a2db75d','53812810-a8e6-4ff7-8dcb-db4b8c2b92ef','b0186a1b-9637-4b48-8524-ab4ff53932e7') ON CONFLICT DO NOTHING

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('719d4066-f194-479f-b501-62277a2db75d','f4788793-b620-424b-b724-7ec56796d43e','ec44ca69-ee85-4aaf-9740-73b2a6350b5a') ON CONFLICT DO NOTHING

UPDATE provinces SET coastal=true WHERE id='ec44ca69-ee85-4aaf-9740-73b2a6350b5a'; INSERT INTO buildings(settlement_id,building_type,level,god_id) VALUES('79aeacfc-c9ac-48d9-a62b-cc1689e23d9b','harbour',1,NULL),('79aeacfc-c9ac-48d9-a62b-cc1689e23d9b','temple',3,'potnia') ON CONFLICT DO NOTHING; UPDATE player_world_records SET kharis_amount=90,kharis_rate=0,kharis_calc_tick=100 WHERE world_id='719d4066-f194-479f-b501-62277a2db75d' AND player_id='53812810-a8e6-4ff7-8dcb-db4b8c2b92ef'; UPDATE settlement_goods SET amount=10000,rate=0,cap=1000000,calc_tick=100 WHERE settlement_id IN('79aeacfc-c9ac-48d9-a62b-cc1689e23d9b','4af82da9-ecba-4dc4-ade0-b5733285bedc'); DELETE FROM settlement_placement WHERE settlement_id='79aeacfc-c9ac-48d9-a62b-cc1689e23d9b'; INSERT INTO settlement_placement(settlement_id,gubbe_ordinal,target_kind,hex_q,hex_r,good_key) VALUES('79aeacfc-c9ac-48d9-a62b-cc1689e23d9b',1,'hex',0,55,'fish');

INSERT INTO sea_storms(world_id,heading,created_tick) VALUES('719d4066-f194-479f-b501-62277a2db75d',0,100) RETURNING id

INSERT INTO sea_storm_track(storm_id,tick,slot,q,r) VALUES('8e5a0178-0b28-42c2-98a3-4afcd92390b8',100,0,0,55),('8e5a0178-0b28-42c2-98a3-4afcd92390b8',100,1,1,55),('8e5a0178-0b28-42c2-98a3-4afcd92390b8',100,2,0,56);

UPDATE scheduled_events SET due_tick=100,process_after=now() WHERE world_id='719d4066-f194-479f-b501-62277a2db75d' AND event_type='MessengerArrival' AND payload->>'messenger_id'='09e8d93a-5abf-442f-863a-f13d9e597177';

UPDATE map_tiles SET copper_deposit=false,tin_deposit=false,silver_deposit=false,incense_deposit=false WHERE world_id='719d4066-f194-479f-b501-62277a2db75d'; UPDATE map_tiles SET terrain='hills',silver_deposit=true WHERE world_id='719d4066-f194-479f-b501-62277a2db75d' AND q=2 AND r=28;

UPDATE settlement_goods SET amount=CASE WHEN good_key='grain' THEN 29 WHEN good_key IN('fish','oil','wine','livestock') THEN 0 ELSE 10000 END,rate=0,calc_tick=100 WHERE settlement_id='79aeacfc-c9ac-48d9-a62b-cc1689e23d9b';

DELETE FROM prayer_cooldowns WHERE world_id='719d4066-f194-479f-b501-62277a2db75d' AND player_id='53812810-a8e6-4ff7-8dcb-db4b8c2b92ef'; DELETE FROM player_scouted_tiles WHERE world_id='719d4066-f194-479f-b501-62277a2db75d' AND player_id='53812810-a8e6-4ff7-8dcb-db4b8c2b92ef' AND (abs(q-2)+abs(q+r-2-28)+abs(r-28))/2<=2;

UPDATE settlement_goods SET amount=1000,rate=0,calc_tick=100 WHERE settlement_id='79aeacfc-c9ac-48d9-a62b-cc1689e23d9b' AND good_key IN('grain','silver');

UPDATE settlement_goods SET amount=CASE WHEN good_key='grain' THEN 29 WHEN good_key IN('fish','oil','wine','livestock') THEN 0 ELSE 10000 END,rate=0,calc_tick=100 WHERE settlement_id='79aeacfc-c9ac-48d9-a62b-cc1689e23d9b';

DELETE FROM prayer_cooldowns WHERE world_id='719d4066-f194-479f-b501-62277a2db75d' AND player_id='53812810-a8e6-4ff7-8dcb-db4b8c2b92ef'; DELETE FROM player_scouted_tiles WHERE world_id='719d4066-f194-479f-b501-62277a2db75d' AND player_id='53812810-a8e6-4ff7-8dcb-db4b8c2b92ef' AND (abs(q-2)+abs(q+r-2-28)+abs(r-28))/2<=2;

UPDATE settlement_goods SET amount=1000,rate=0,calc_tick=100 WHERE settlement_id='79aeacfc-c9ac-48d9-a62b-cc1689e23d9b' AND good_key IN('grain','silver');
