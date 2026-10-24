UPDATE players SET wanax_name='Leandros' WHERE id='54e3be83-1a91-4a30-b5a6-83170277d0b6'

UPDATE players SET wanax_name='Myrine' WHERE id='9f0b3508-1de9-4cb9-8c89-0abfb259784e'

UPDATE players SET wanax_name='Dorian' WHERE id='fe68a3f3-317a-4a6d-a8f5-59ff16badbdc'

UPDATE map_tiles SET terrain='plains',coastal=true WHERE world_id='f0feeb47-3f73-44e7-a707-240eb29bf9f0' AND r=29 AND q BETWEEN 1 AND 7; UPDATE map_tiles SET terrain='coastal_sea' WHERE world_id='f0feeb47-3f73-44e7-a707-240eb29bf9f0' AND q=0 AND r=30;

UPDATE units SET q=1,r=29,status='positioned' WHERE id='2f27050f-7b5c-473c-8700-bd66327f9dfe';

UPDATE units SET q=7,r=29,status='positioned' WHERE id='757d30ac-6ed8-4cb8-8ced-7ffe423f3af6';

UPDATE units SET q=3,r=29,status='positioned' WHERE id='dcba233b-7b07-461f-8435-6749271a69cd'; UPDATE founder_phase SET is_refugee=true WHERE host_unit_id='dcba233b-7b07-461f-8435-6749271a69cd'; UPDATE worlds SET current_tick=100,last_tick_at=now() WHERE id='f0feeb47-3f73-44e7-a707-240eb29bf9f0';

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('f0feeb47-3f73-44e7-a707-240eb29bf9f0','54e3be83-1a91-4a30-b5a6-83170277d0b6','8f3243ef-aa6d-4db3-a183-8b02797b2f4d') ON CONFLICT DO NOTHING

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('f0feeb47-3f73-44e7-a707-240eb29bf9f0','9f0b3508-1de9-4cb9-8c89-0abfb259784e','340e5056-97e1-4140-9f0d-f3387be19083') ON CONFLICT DO NOTHING

UPDATE provinces SET coastal=true WHERE id='340e5056-97e1-4140-9f0d-f3387be19083'; INSERT INTO buildings(settlement_id,building_type,level,god_id) VALUES('ed44cc8e-1a85-4249-b8a7-37f232aabf62','harbour',1,NULL),('ed44cc8e-1a85-4249-b8a7-37f232aabf62','temple',3,'potnia') ON CONFLICT DO NOTHING; UPDATE player_world_records SET kharis_amount=90,kharis_rate=0,kharis_calc_tick=100 WHERE world_id='f0feeb47-3f73-44e7-a707-240eb29bf9f0' AND player_id='54e3be83-1a91-4a30-b5a6-83170277d0b6'; UPDATE settlement_goods SET amount=10000,rate=0,cap=1000000,calc_tick=100 WHERE settlement_id IN('ed44cc8e-1a85-4249-b8a7-37f232aabf62','2e7a7f8b-331b-4961-b738-bd38366a1479'); DELETE FROM settlement_placement WHERE settlement_id='ed44cc8e-1a85-4249-b8a7-37f232aabf62'; INSERT INTO settlement_placement(settlement_id,gubbe_ordinal,target_kind,hex_q,hex_r,good_key) VALUES('ed44cc8e-1a85-4249-b8a7-37f232aabf62',1,'hex',0,30,'fish');

INSERT INTO sea_storms(world_id,heading,created_tick) VALUES('f0feeb47-3f73-44e7-a707-240eb29bf9f0',0,100) RETURNING id

INSERT INTO sea_storm_track(storm_id,tick,slot,q,r) VALUES('112bf34a-b1f7-4ce1-b4d9-0ddbe6c6843f',100,0,0,30);

UPDATE scheduled_events SET due_tick=100,process_after=now() WHERE world_id='f0feeb47-3f73-44e7-a707-240eb29bf9f0' AND event_type='MessengerArrival' AND payload->>'messenger_id'='543cdf27-c316-4638-b430-eff412881a78';

UPDATE map_tiles SET copper_deposit=false,tin_deposit=false,silver_deposit=false WHERE world_id='f0feeb47-3f73-44e7-a707-240eb29bf9f0'; UPDATE map_tiles SET terrain='hills',silver_deposit=true WHERE world_id='f0feeb47-3f73-44e7-a707-240eb29bf9f0' AND q=1 AND r=53;

UPDATE settlement_goods SET amount=CASE WHEN good_key='grain' THEN 29 WHEN good_key IN('fish','oil','wine') THEN 0 ELSE 10000 END,rate=0,calc_tick=100 WHERE settlement_id='ed44cc8e-1a85-4249-b8a7-37f232aabf62';

DELETE FROM prayer_cooldowns WHERE world_id='f0feeb47-3f73-44e7-a707-240eb29bf9f0' AND player_id='54e3be83-1a91-4a30-b5a6-83170277d0b6'; DELETE FROM player_scouted_tiles WHERE world_id='f0feeb47-3f73-44e7-a707-240eb29bf9f0' AND player_id='54e3be83-1a91-4a30-b5a6-83170277d0b6' AND (abs(q-1)+abs(q+r-1-53)+abs(r-53))/2<=1;

UPDATE settlement_goods SET amount=1000,rate=0,calc_tick=100 WHERE settlement_id='ed44cc8e-1a85-4249-b8a7-37f232aabf62' AND good_key IN('grain','silver');

UPDATE settlement_goods SET amount=CASE WHEN good_key='grain' THEN 29 WHEN good_key IN('fish','oil','wine') THEN 0 ELSE 10000 END,rate=0,calc_tick=100 WHERE settlement_id='ed44cc8e-1a85-4249-b8a7-37f232aabf62';

DELETE FROM prayer_cooldowns WHERE world_id='f0feeb47-3f73-44e7-a707-240eb29bf9f0' AND player_id='54e3be83-1a91-4a30-b5a6-83170277d0b6'; DELETE FROM player_scouted_tiles WHERE world_id='f0feeb47-3f73-44e7-a707-240eb29bf9f0' AND player_id='54e3be83-1a91-4a30-b5a6-83170277d0b6' AND (abs(q-1)+abs(q+r-1-53)+abs(r-53))/2<=1;

UPDATE settlement_goods SET amount=1000,rate=0,calc_tick=100 WHERE settlement_id='ed44cc8e-1a85-4249-b8a7-37f232aabf62' AND good_key IN('grain','silver');
