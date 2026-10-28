UPDATE players SET wanax_name='Leandros' WHERE id='e7821db1-6a38-42ae-bbef-4876261c7b55'

UPDATE players SET wanax_name='Myrine' WHERE id='86ef067a-e149-4be7-8f8a-57b0647ad79e'

UPDATE players SET wanax_name='Dorian' WHERE id='f6f41d1a-7230-454e-9695-722dfb5424eb'

UPDATE map_tiles SET terrain='plains',coastal=true WHERE world_id='dacc7199-f89f-4a2c-b3e3-0dca52d91f47' AND r=36 AND q BETWEEN 1 AND 7; UPDATE map_tiles SET terrain='coastal_sea' WHERE world_id='dacc7199-f89f-4a2c-b3e3-0dca52d91f47' AND (q,r) IN((0,37),(1,37),(0,38));

UPDATE units SET q=1,r=36,status='positioned' WHERE id='bf21b08c-48a1-450b-83d8-a5133f729a92';

UPDATE units SET q=1,r=36,status='garrison',settlement_id='219c0ed6-2f2c-4dc8-a8c3-dfce81bb9b7c',support_settlement_id='219c0ed6-2f2c-4dc8-a8c3-dfce81bb9b7c' WHERE world_id='dacc7199-f89f-4a2c-b3e3-0dca52d91f47' AND owner_id='e7821db1-6a38-42ae-bbef-4876261c7b55' AND type='spearman';

UPDATE units SET q=7,r=36,status='positioned' WHERE id='3581c05b-b74e-4486-8dee-e6c80132d8e5';

UPDATE units SET q=7,r=36,status='garrison',settlement_id='c9b68c41-af3a-42b5-800c-bbbe0f8708cd',support_settlement_id='c9b68c41-af3a-42b5-800c-bbbe0f8708cd' WHERE world_id='dacc7199-f89f-4a2c-b3e3-0dca52d91f47' AND owner_id='86ef067a-e149-4be7-8f8a-57b0647ad79e' AND type='spearman';

UPDATE units SET q=3,r=36,status='positioned' WHERE id='45ec7b93-78b7-4758-a49b-8e1c623a66fd'; UPDATE founder_phase SET is_refugee=true WHERE host_unit_id='45ec7b93-78b7-4758-a49b-8e1c623a66fd'; UPDATE worlds SET current_tick=100,last_tick_at=now() WHERE id='dacc7199-f89f-4a2c-b3e3-0dca52d91f47';

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('dacc7199-f89f-4a2c-b3e3-0dca52d91f47','e7821db1-6a38-42ae-bbef-4876261c7b55','06d16bfc-f22a-406b-ae3b-83cfc9e583f2') ON CONFLICT DO NOTHING

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('dacc7199-f89f-4a2c-b3e3-0dca52d91f47','86ef067a-e149-4be7-8f8a-57b0647ad79e','84176ba2-98ee-41a3-a20f-09c644ffa716') ON CONFLICT DO NOTHING

UPDATE provinces SET coastal=true WHERE id='84176ba2-98ee-41a3-a20f-09c644ffa716'; INSERT INTO buildings(settlement_id,building_type,level,god_id) VALUES('219c0ed6-2f2c-4dc8-a8c3-dfce81bb9b7c','harbour',1,NULL),('219c0ed6-2f2c-4dc8-a8c3-dfce81bb9b7c','temple',3,'potnia') ON CONFLICT DO NOTHING; UPDATE player_world_records SET kharis_amount=90,kharis_rate=0,kharis_calc_tick=100 WHERE world_id='dacc7199-f89f-4a2c-b3e3-0dca52d91f47' AND player_id='e7821db1-6a38-42ae-bbef-4876261c7b55'; UPDATE settlement_goods SET amount=10000,rate=0,cap=1000000,calc_tick=100 WHERE settlement_id IN('219c0ed6-2f2c-4dc8-a8c3-dfce81bb9b7c','c9b68c41-af3a-42b5-800c-bbbe0f8708cd'); DELETE FROM settlement_placement WHERE settlement_id='219c0ed6-2f2c-4dc8-a8c3-dfce81bb9b7c'; INSERT INTO settlement_placement(settlement_id,gubbe_ordinal,target_kind,hex_q,hex_r,good_key) VALUES('219c0ed6-2f2c-4dc8-a8c3-dfce81bb9b7c',1,'hex',0,37,'fish');

INSERT INTO sea_storms(world_id,heading,created_tick) VALUES('dacc7199-f89f-4a2c-b3e3-0dca52d91f47',0,100) RETURNING id

INSERT INTO sea_storm_track(storm_id,tick,slot,q,r) VALUES('a6159979-37cd-4bc0-9095-4fdc44f9427a',100,0,0,37),('a6159979-37cd-4bc0-9095-4fdc44f9427a',100,1,1,37),('a6159979-37cd-4bc0-9095-4fdc44f9427a',100,2,0,38);

UPDATE map_tiles SET terrain='coastal_sea' WHERE world_id='dacc7199-f89f-4a2c-b3e3-0dca52d91f47' AND q=1 AND r=38; INSERT INTO sea_storm_track(storm_id,tick,slot,q,r) VALUES('a6159979-37cd-4bc0-9095-4fdc44f9427a',101,0,1,37),('a6159979-37cd-4bc0-9095-4fdc44f9427a',101,1,0,38),('a6159979-37cd-4bc0-9095-4fdc44f9427a',101,2,1,38);

INSERT INTO sea_storm_track(storm_id,tick,slot,q,r) VALUES('a6159979-37cd-4bc0-9095-4fdc44f9427a',102,0,0,37),('a6159979-37cd-4bc0-9095-4fdc44f9427a',102,1,1,37),('a6159979-37cd-4bc0-9095-4fdc44f9427a',102,2,0,38);
