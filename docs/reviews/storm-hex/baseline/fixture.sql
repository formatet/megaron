UPDATE players SET wanax_name='Leandros' WHERE id='4874f90d-4cdd-4730-9370-56080884fd12'

UPDATE players SET wanax_name='Myrine' WHERE id='346f31d6-ff65-4d9b-99be-4dde55ba7575'

UPDATE players SET wanax_name='Dorian' WHERE id='e031e217-2554-4c33-a1e8-35331c7a6088'

UPDATE map_tiles SET terrain='plains',coastal=true WHERE world_id='a7566a25-8077-4186-89b7-793cc21d6daf' AND r=27 AND q BETWEEN 3 AND 9; UPDATE map_tiles SET terrain='coastal_sea' WHERE world_id='a7566a25-8077-4186-89b7-793cc21d6daf' AND (q,r) IN((2,28),(3,28),(2,29));

UPDATE units SET q=3,r=27,status='positioned' WHERE id='efd381f6-d05c-4270-a887-a9cde100b244';

UPDATE units SET q=3,r=27,status='garrison',settlement_id='63d9e9d9-d3ba-44f9-9cf8-3048315908d4',support_settlement_id='63d9e9d9-d3ba-44f9-9cf8-3048315908d4' WHERE world_id='a7566a25-8077-4186-89b7-793cc21d6daf' AND owner_id='4874f90d-4cdd-4730-9370-56080884fd12' AND type='spearman';

UPDATE units SET q=9,r=27,status='positioned' WHERE id='eb0b369c-d8e5-449d-9771-74e7d51e809c';

UPDATE units SET q=9,r=27,status='garrison',settlement_id='437ba6ce-5bd8-49a7-aed5-6357a0096a0f',support_settlement_id='437ba6ce-5bd8-49a7-aed5-6357a0096a0f' WHERE world_id='a7566a25-8077-4186-89b7-793cc21d6daf' AND owner_id='346f31d6-ff65-4d9b-99be-4dde55ba7575' AND type='spearman';

UPDATE units SET q=5,r=27,status='positioned' WHERE id='bcd45930-4aae-494c-8fba-691dc1544599'; UPDATE founder_phase SET is_refugee=true WHERE host_unit_id='bcd45930-4aae-494c-8fba-691dc1544599'; UPDATE worlds SET current_tick=100,last_tick_at=now() WHERE id='a7566a25-8077-4186-89b7-793cc21d6daf';

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('a7566a25-8077-4186-89b7-793cc21d6daf','4874f90d-4cdd-4730-9370-56080884fd12','80956ff9-d726-4d31-a3c0-fa36a98173a5') ON CONFLICT DO NOTHING

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('a7566a25-8077-4186-89b7-793cc21d6daf','346f31d6-ff65-4d9b-99be-4dde55ba7575','5828af07-49a2-4034-9c5b-71d45613a5fc') ON CONFLICT DO NOTHING

UPDATE provinces SET coastal=true WHERE id='5828af07-49a2-4034-9c5b-71d45613a5fc'; INSERT INTO buildings(settlement_id,building_type,level,god_id) VALUES('63d9e9d9-d3ba-44f9-9cf8-3048315908d4','harbour',1,NULL),('63d9e9d9-d3ba-44f9-9cf8-3048315908d4','temple',3,'potnia') ON CONFLICT DO NOTHING; UPDATE player_world_records SET kharis_amount=90,kharis_rate=0,kharis_calc_tick=100 WHERE world_id='a7566a25-8077-4186-89b7-793cc21d6daf' AND player_id='4874f90d-4cdd-4730-9370-56080884fd12'; UPDATE settlement_goods SET amount=10000,rate=0,cap=1000000,calc_tick=100 WHERE settlement_id IN('63d9e9d9-d3ba-44f9-9cf8-3048315908d4','437ba6ce-5bd8-49a7-aed5-6357a0096a0f'); DELETE FROM settlement_placement WHERE settlement_id='63d9e9d9-d3ba-44f9-9cf8-3048315908d4'; INSERT INTO settlement_placement(settlement_id,gubbe_ordinal,target_kind,hex_q,hex_r,good_key) VALUES('63d9e9d9-d3ba-44f9-9cf8-3048315908d4',1,'hex',2,28,'fish');

INSERT INTO sea_storms(world_id,heading,created_tick) VALUES('a7566a25-8077-4186-89b7-793cc21d6daf',0,100) RETURNING id

INSERT INTO sea_storm_track(storm_id,tick,slot,q,r) VALUES('6f1ad16e-760c-46a2-b24e-faf9cc1829c5',100,0,2,28),('6f1ad16e-760c-46a2-b24e-faf9cc1829c5',100,1,3,28),('6f1ad16e-760c-46a2-b24e-faf9cc1829c5',100,2,2,29);

UPDATE map_tiles SET terrain='coastal_sea' WHERE world_id='a7566a25-8077-4186-89b7-793cc21d6daf' AND q=3 AND r=29; INSERT INTO sea_storm_track(storm_id,tick,slot,q,r) VALUES('6f1ad16e-760c-46a2-b24e-faf9cc1829c5',101,0,3,28),('6f1ad16e-760c-46a2-b24e-faf9cc1829c5',101,1,2,29),('6f1ad16e-760c-46a2-b24e-faf9cc1829c5',101,2,3,29);

INSERT INTO sea_storm_track(storm_id,tick,slot,q,r) VALUES('6f1ad16e-760c-46a2-b24e-faf9cc1829c5',102,0,2,28),('6f1ad16e-760c-46a2-b24e-faf9cc1829c5',102,1,3,28),('6f1ad16e-760c-46a2-b24e-faf9cc1829c5',102,2,2,29);
