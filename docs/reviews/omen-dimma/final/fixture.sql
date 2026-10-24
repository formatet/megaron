UPDATE players SET wanax_name='Leandros' WHERE id='609f200d-f937-46a0-84b9-da7cc55391ad'

UPDATE players SET wanax_name='Myrine' WHERE id='3fa82b69-a49d-41b9-9393-132440ddb129'

UPDATE players SET wanax_name='Dorian' WHERE id='0e38a18a-f448-4034-bcfa-5b40075de68d'

UPDATE map_tiles SET terrain='plains',coastal=true WHERE world_id='591f4318-d069-44a4-aed8-c77cab5d5c7d' AND r=47 AND q BETWEEN 1 AND 7; UPDATE map_tiles SET terrain='coastal_sea' WHERE world_id='591f4318-d069-44a4-aed8-c77cab5d5c7d' AND (q,r) IN((0,48),(1,48),(0,49));

UPDATE units SET q=1,r=47,status='positioned' WHERE id='ba802ecd-e75f-4431-aed1-de9be9af7284';

UPDATE units SET q=1,r=47,status='garrison',settlement_id='f2586b98-a4ac-49ae-9612-2845bb2009e9',support_settlement_id='f2586b98-a4ac-49ae-9612-2845bb2009e9' WHERE world_id='591f4318-d069-44a4-aed8-c77cab5d5c7d' AND owner_id='609f200d-f937-46a0-84b9-da7cc55391ad' AND type='spearman';

UPDATE units SET q=7,r=47,status='positioned' WHERE id='83469bbd-4104-49d5-9158-a6a912d8be0a';

UPDATE units SET q=7,r=47,status='garrison',settlement_id='9cca8f64-419c-430c-a2c2-b17dfc1427f0',support_settlement_id='9cca8f64-419c-430c-a2c2-b17dfc1427f0' WHERE world_id='591f4318-d069-44a4-aed8-c77cab5d5c7d' AND owner_id='3fa82b69-a49d-41b9-9393-132440ddb129' AND type='spearman';

UPDATE units SET q=3,r=47,status='positioned' WHERE id='41111293-4ddb-4f36-a6c0-16339a2fa96d'; UPDATE founder_phase SET is_refugee=true WHERE host_unit_id='41111293-4ddb-4f36-a6c0-16339a2fa96d'; UPDATE worlds SET current_tick=100,last_tick_at=now() WHERE id='591f4318-d069-44a4-aed8-c77cab5d5c7d';

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('591f4318-d069-44a4-aed8-c77cab5d5c7d','609f200d-f937-46a0-84b9-da7cc55391ad','9a591eb2-8660-4f08-ba76-591e70db4a04') ON CONFLICT DO NOTHING

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('591f4318-d069-44a4-aed8-c77cab5d5c7d','3fa82b69-a49d-41b9-9393-132440ddb129','600acc87-3b39-4ff5-a513-b75e049ee5b4') ON CONFLICT DO NOTHING

UPDATE provinces SET coastal=true WHERE id='600acc87-3b39-4ff5-a513-b75e049ee5b4'; INSERT INTO buildings(settlement_id,building_type,level,god_id) VALUES('f2586b98-a4ac-49ae-9612-2845bb2009e9','harbour',1,NULL),('f2586b98-a4ac-49ae-9612-2845bb2009e9','temple',3,'potnia') ON CONFLICT DO NOTHING; UPDATE player_world_records SET kharis_amount=90,kharis_rate=0,kharis_calc_tick=100 WHERE world_id='591f4318-d069-44a4-aed8-c77cab5d5c7d' AND player_id='609f200d-f937-46a0-84b9-da7cc55391ad'; UPDATE settlement_goods SET amount=10000,rate=0,cap=1000000,calc_tick=100 WHERE settlement_id IN('f2586b98-a4ac-49ae-9612-2845bb2009e9','9cca8f64-419c-430c-a2c2-b17dfc1427f0'); DELETE FROM settlement_placement WHERE settlement_id='f2586b98-a4ac-49ae-9612-2845bb2009e9'; INSERT INTO settlement_placement(settlement_id,gubbe_ordinal,target_kind,hex_q,hex_r,good_key) VALUES('f2586b98-a4ac-49ae-9612-2845bb2009e9',1,'hex',0,48,'fish');

INSERT INTO sea_storms(world_id,heading,created_tick) VALUES('591f4318-d069-44a4-aed8-c77cab5d5c7d',0,100) RETURNING id

INSERT INTO sea_storm_track(storm_id,tick,slot,q,r) VALUES('4da6b491-ee82-4707-b240-e45dfd2df2b9',100,0,0,48),('4da6b491-ee82-4707-b240-e45dfd2df2b9',100,1,1,48),('4da6b491-ee82-4707-b240-e45dfd2df2b9',100,2,0,49);

UPDATE scheduled_events SET due_tick=100,process_after=now() WHERE world_id='591f4318-d069-44a4-aed8-c77cab5d5c7d' AND event_type='MessengerArrival' AND payload->>'messenger_id'='cf3bf738-7cf7-40ce-a6f8-5c5f739916e0';

UPDATE map_tiles SET copper_deposit=false,tin_deposit=false,silver_deposit=false,incense_deposit=false WHERE world_id='591f4318-d069-44a4-aed8-c77cab5d5c7d'; UPDATE map_tiles SET terrain='hills',silver_deposit=true WHERE world_id='591f4318-d069-44a4-aed8-c77cab5d5c7d' AND q=2 AND r=28;

DELETE FROM prayer_cooldowns WHERE world_id='591f4318-d069-44a4-aed8-c77cab5d5c7d' AND player_id='609f200d-f937-46a0-84b9-da7cc55391ad'; DELETE FROM player_scouted_tiles WHERE world_id='591f4318-d069-44a4-aed8-c77cab5d5c7d' AND player_id='609f200d-f937-46a0-84b9-da7cc55391ad' AND (abs(q-2)+abs(q+r-2-28)+abs(r-28))/2<=2; INSERT INTO player_scouted_tiles(world_id,player_id,q,r) VALUES('591f4318-d069-44a4-aed8-c77cab5d5c7d','609f200d-f937-46a0-84b9-da7cc55391ad',3,28) ON CONFLICT DO NOTHING; UPDATE settlement_goods SET amount=10000,rate=0,calc_tick=100 WHERE settlement_id='f2586b98-a4ac-49ae-9612-2845bb2009e9' AND good_key IN('grain','silver'); UPDATE player_world_records SET kharis_amount=90,kharis_calc_tick=100 WHERE player_id='609f200d-f937-46a0-84b9-da7cc55391ad' AND world_id='591f4318-d069-44a4-aed8-c77cab5d5c7d';

DELETE FROM prayer_cooldowns WHERE world_id='591f4318-d069-44a4-aed8-c77cab5d5c7d' AND player_id='609f200d-f937-46a0-84b9-da7cc55391ad'; DELETE FROM player_scouted_tiles WHERE world_id='591f4318-d069-44a4-aed8-c77cab5d5c7d' AND player_id='609f200d-f937-46a0-84b9-da7cc55391ad' AND (abs(q-2)+abs(q+r-2-28)+abs(r-28))/2<=2; INSERT INTO player_scouted_tiles(world_id,player_id,q,r) VALUES('591f4318-d069-44a4-aed8-c77cab5d5c7d','609f200d-f937-46a0-84b9-da7cc55391ad',3,28) ON CONFLICT DO NOTHING; UPDATE settlement_goods SET amount=10000,rate=0,calc_tick=100 WHERE settlement_id='f2586b98-a4ac-49ae-9612-2845bb2009e9' AND good_key IN('grain','silver'); UPDATE player_world_records SET kharis_amount=90,kharis_calc_tick=100 WHERE player_id='609f200d-f937-46a0-84b9-da7cc55391ad' AND world_id='591f4318-d069-44a4-aed8-c77cab5d5c7d';
