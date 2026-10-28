UPDATE players SET wanax_name='Leandros' WHERE id='6af13b57-53d6-4589-83a5-11ebb90f596e'

UPDATE players SET wanax_name='Myrine' WHERE id='d96dc9ce-de55-4be5-9dfd-d719f0790d55'

UPDATE players SET wanax_name='Dorian' WHERE id='5e804afa-0941-4d06-a621-ce50570ff202'

UPDATE map_tiles SET terrain='plains',coastal=true WHERE world_id='6d25af18-f7df-41bc-a689-4991c316aca7' AND r=48 AND q BETWEEN 1 AND 7; UPDATE map_tiles SET terrain='coastal_sea' WHERE world_id='6d25af18-f7df-41bc-a689-4991c316aca7' AND (q,r) IN((0,49),(1,49),(0,50));

UPDATE units SET q=1,r=48,status='positioned' WHERE id='5c3ca4a8-6abf-455b-a6a5-096d7939519c';

UPDATE units SET q=1,r=48,status='garrison',settlement_id='f13a4b78-1066-4682-98bd-174177b989c8',support_settlement_id='f13a4b78-1066-4682-98bd-174177b989c8' WHERE world_id='6d25af18-f7df-41bc-a689-4991c316aca7' AND owner_id='6af13b57-53d6-4589-83a5-11ebb90f596e' AND type='spearman';

UPDATE units SET q=7,r=48,status='positioned' WHERE id='fdd7455f-4f53-4e1f-bd3e-be695d9123cb';

UPDATE units SET q=7,r=48,status='garrison',settlement_id='d4d405a7-9649-45f7-9484-cc65dd7d66c8',support_settlement_id='d4d405a7-9649-45f7-9484-cc65dd7d66c8' WHERE world_id='6d25af18-f7df-41bc-a689-4991c316aca7' AND owner_id='d96dc9ce-de55-4be5-9dfd-d719f0790d55' AND type='spearman';

UPDATE units SET q=3,r=48,status='positioned' WHERE id='4e07c297-068f-4fad-9a70-a07a736146ac'; UPDATE founder_phase SET is_refugee=true WHERE host_unit_id='4e07c297-068f-4fad-9a70-a07a736146ac'; UPDATE worlds SET current_tick=100,last_tick_at=now() WHERE id='6d25af18-f7df-41bc-a689-4991c316aca7';

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('6d25af18-f7df-41bc-a689-4991c316aca7','6af13b57-53d6-4589-83a5-11ebb90f596e','4295cb18-d62a-493d-ab80-663768fe2952') ON CONFLICT DO NOTHING

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('6d25af18-f7df-41bc-a689-4991c316aca7','d96dc9ce-de55-4be5-9dfd-d719f0790d55','c2ae6f04-1a8d-4455-a813-b44832eaa6bb') ON CONFLICT DO NOTHING

UPDATE provinces SET coastal=true WHERE id='c2ae6f04-1a8d-4455-a813-b44832eaa6bb'; INSERT INTO buildings(settlement_id,building_type,level,god_id) VALUES('f13a4b78-1066-4682-98bd-174177b989c8','harbour',1,NULL),('f13a4b78-1066-4682-98bd-174177b989c8','temple',3,'potnia') ON CONFLICT DO NOTHING; UPDATE player_world_records SET kharis_amount=90,kharis_rate=0,kharis_calc_tick=100 WHERE world_id='6d25af18-f7df-41bc-a689-4991c316aca7' AND player_id='6af13b57-53d6-4589-83a5-11ebb90f596e'; UPDATE settlement_goods SET amount=10000,rate=0,cap=1000000,calc_tick=100 WHERE settlement_id IN('f13a4b78-1066-4682-98bd-174177b989c8','d4d405a7-9649-45f7-9484-cc65dd7d66c8'); DELETE FROM settlement_placement WHERE settlement_id='f13a4b78-1066-4682-98bd-174177b989c8'; INSERT INTO settlement_placement(settlement_id,gubbe_ordinal,target_kind,hex_q,hex_r,good_key) VALUES('f13a4b78-1066-4682-98bd-174177b989c8',1,'hex',0,49,'fish');

INSERT INTO sea_storms(world_id,heading,created_tick) VALUES('6d25af18-f7df-41bc-a689-4991c316aca7',0,100) RETURNING id

INSERT INTO sea_storm_track(storm_id,tick,slot,q,r) VALUES('b400a54b-a6cd-44e3-84d5-20534d535775',100,0,0,49),('b400a54b-a6cd-44e3-84d5-20534d535775',100,1,1,49),('b400a54b-a6cd-44e3-84d5-20534d535775',100,2,0,50);

UPDATE map_tiles SET terrain='coastal_sea' WHERE world_id='6d25af18-f7df-41bc-a689-4991c316aca7' AND q=1 AND r=50; INSERT INTO sea_storm_track(storm_id,tick,slot,q,r) VALUES('b400a54b-a6cd-44e3-84d5-20534d535775',101,0,1,49),('b400a54b-a6cd-44e3-84d5-20534d535775',101,1,0,50),('b400a54b-a6cd-44e3-84d5-20534d535775',101,2,1,50);

INSERT INTO sea_storm_track(storm_id,tick,slot,q,r) VALUES('b400a54b-a6cd-44e3-84d5-20534d535775',102,0,0,49),('b400a54b-a6cd-44e3-84d5-20534d535775',102,1,1,49),('b400a54b-a6cd-44e3-84d5-20534d535775',102,2,0,50);
