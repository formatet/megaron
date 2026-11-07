# Bildsvep 2026-11-07

Rigg: `tools/bildsvep_20261107.py` (egna docker-containrar för postgres+redis på slumpad port, server byggd från master `e7e01906`, Firefox via Playwright, vy 1280x900 och 390x844, zoom 1,0).
Ingen kod i `server/` eller `web/` är ändrad; live och acceptance-containern är orörda. Världen är 56x40 med en preparerad slätt (rad r-3..r) och kustvatten söder därom; städer: Phaistos (Idomeneus, spelaren), Gournia (Myrine), Pylos (Nestor), Kydonia (Atreus), Knossos (Minos, observatör som ser hela kartan).
Där `tall` användes i skriptet förstorades fönstrets höjd tillfälligt till 1500 px för att få med hela panelen i beskärningen; bredden är alltid 1280/390.

Öppna `index.html`. Fixturloggen nedan är exakt de SQL-satser skriptet körde (samma ordning, per scen). Det som inte står här kom från riktiga API-anrop: registrering, join, grundande av städer, brevskick, byggorder, och UI-klick.

## Fixturer per scen

```sql
-- scene world
UPDATE map_tiles SET terrain='plains',coastal=false,river_edges=NULL,river_upstream_edges=NULL,river_downstream_edges=NULL,river_flow_known=NULL WHERE world_id='39addc16-6709-4b1f-a609-ab68f50d7354' AND r BETWEEN 21 AND 24 AND q BETWEEN 3 AND 36; UPDATE map_tiles SET terrain='coastal_sea',coastal=false,river_edges=NULL,river_upstream_edges=NULL,river_downstream_edges=NULL,river_flow_known=NULL WHERE world_id='39addc16-6709-4b1f-a609-ab68f50d7354' AND r BETWEEN 25 AND 27 AND q BETWEEN 3 AND 36; UPDATE map_tiles SET coastal=true WHERE world_id='39addc16-6709-4b1f-a609-ab68f50d7354' AND r=24 AND q BETWEEN 3 AND 36; UPDATE map_tiles SET copper_deposit=false,tin_deposit=false,silver_deposit=false,incense_deposit=false WHERE world_id='39addc16-6709-4b1f-a609-ab68f50d7354' AND r BETWEEN 21 AND 27 AND q BETWEEN 3 AND 36;

-- scene world
UPDATE players SET wanax_name='Idomeneus' WHERE id='374645c6-b550-4a31-9faa-58af0e2e5950'

-- scene world
UPDATE units SET q=6,r=24,status='positioned' WHERE id='6e3b9da0-2a63-448d-9997-70167a2c5ca4';

-- scene world
UPDATE units SET q=6,r=24,status='garrison',settlement_id='54b1a5b5-1d02-48b6-91be-48c5f29b510d',support_settlement_id='54b1a5b5-1d02-48b6-91be-48c5f29b510d' WHERE world_id='39addc16-6709-4b1f-a609-ab68f50d7354' AND owner_id='374645c6-b550-4a31-9faa-58af0e2e5950' AND type='spearman';

-- scene world
UPDATE players SET wanax_name='Myrine' WHERE id='d53f7076-832b-4ba0-a607-8622f6bf7209'

-- scene world
UPDATE units SET q=12,r=24,status='positioned' WHERE id='339ae1ee-5b1d-4ef4-96e6-f15db83cf0d1';

-- scene world
UPDATE units SET q=12,r=24,status='garrison',settlement_id='c9505416-c067-41e7-85c4-58a731372afe',support_settlement_id='c9505416-c067-41e7-85c4-58a731372afe' WHERE world_id='39addc16-6709-4b1f-a609-ab68f50d7354' AND owner_id='d53f7076-832b-4ba0-a607-8622f6bf7209' AND type='spearman';

-- scene world
UPDATE players SET wanax_name='Nestor' WHERE id='1f991980-7b82-4f7a-87d7-788c983001c5'

-- scene world
UPDATE units SET q=20,r=24,status='positioned' WHERE id='bc0a3580-d44a-4ab8-9bf4-35c974768447';

-- scene world
UPDATE units SET q=20,r=24,status='garrison',settlement_id='b756a5e3-d16f-48df-9aba-e2c749a8258f',support_settlement_id='b756a5e3-d16f-48df-9aba-e2c749a8258f' WHERE world_id='39addc16-6709-4b1f-a609-ab68f50d7354' AND owner_id='1f991980-7b82-4f7a-87d7-788c983001c5' AND type='spearman';

-- scene world
UPDATE players SET wanax_name='Atreus' WHERE id='0c874aab-3a7f-4381-907e-694730389f79'

-- scene world
UPDATE units SET q=28,r=24,status='positioned' WHERE id='e3f3f9f6-3e9f-40bd-b6b3-aefa2afa7a8a';

-- scene world
UPDATE units SET q=28,r=24,status='garrison',settlement_id='c46b9ce8-d27a-419b-b770-333058267c44',support_settlement_id='c46b9ce8-d27a-419b-b770-333058267c44' WHERE world_id='39addc16-6709-4b1f-a609-ab68f50d7354' AND owner_id='0c874aab-3a7f-4381-907e-694730389f79' AND type='spearman';

-- scene world
UPDATE players SET wanax_name='Minos' WHERE id='f39e8f61-929c-40d3-9a7b-7a32c1d7ebbd'

-- scene world
UPDATE units SET q=34,r=24,status='positioned' WHERE id='3260c0b0-4f49-4805-885a-cd7684199e31';

-- scene world
UPDATE units SET q=34,r=24,status='garrison',settlement_id='3c31a9f5-d3ac-45ad-aa53-604819bab708',support_settlement_id='3c31a9f5-d3ac-45ad-aa53-604819bab708' WHERE world_id='39addc16-6709-4b1f-a609-ab68f50d7354' AND owner_id='f39e8f61-929c-40d3-9a7b-7a32c1d7ebbd' AND type='spearman';

-- scene world
UPDATE worlds SET current_tick=100,last_tick_at=now() WHERE id='39addc16-6709-4b1f-a609-ab68f50d7354'

-- scene world
UPDATE settlement_goods SET amount=10000,rate=0,cap=1000000,calc_tick=100 WHERE settlement_id='54b1a5b5-1d02-48b6-91be-48c5f29b510d'

-- scene B1
UPDATE settlement_goods SET rate=0,calc_tick=100,amount=CASE good_key WHEN 'grain' THEN 3200 WHEN 'fish' THEN 800 WHEN 'oil' THEN 150 WHEN 'timber' THEN 340 WHEN 'silver' THEN 1200 WHEN 'copper' THEN 120 WHEN 'tin' THEN 60 WHEN 'bronze' THEN 45 WHEN 'pottery' THEN 24 WHEN 'horses' THEN 8 WHEN 'livestock' THEN 40 WHEN 'stone' THEN 200 WHEN 'wine' THEN 60 WHEN 'cult' THEN 0 ELSE 0 END WHERE settlement_id='54b1a5b5-1d02-48b6-91be-48c5f29b510d'; INSERT INTO buildings(settlement_id,building_type,level) VALUES('54b1a5b5-1d02-48b6-91be-48c5f29b510d','market',1),('54b1a5b5-1d02-48b6-91be-48c5f29b510d','barracks',1),('54b1a5b5-1d02-48b6-91be-48c5f29b510d','stable',1),('54b1a5b5-1d02-48b6-91be-48c5f29b510d','silo',1),('54b1a5b5-1d02-48b6-91be-48c5f29b510d','pen',1),('54b1a5b5-1d02-48b6-91be-48c5f29b510d','harbour',1) ON CONFLICT DO NOTHING; UPDATE provinces SET coastal=true WHERE id='801b0e58-a7ae-467c-8dfc-54ba46b8a07c'; INSERT INTO market_boards(settlement_id,world_id,buys,sells,written_tick) VALUES('54b1a5b5-1d02-48b6-91be-48c5f29b510d','39addc16-6709-4b1f-a609-ab68f50d7354','{tin,copper}','{pottery,oil}',90) ON CONFLICT DO NOTHING; INSERT INTO player_scouted_tiles(world_id,player_id,q,r) SELECT '39addc16-6709-4b1f-a609-ab68f50d7354','374645c6-b550-4a31-9faa-58af0e2e5950',q,r FROM map_tiles WHERE world_id='39addc16-6709-4b1f-a609-ab68f50d7354' AND (abs(q-6)+abs(q+r-6-24)+abs(r-24))/2<=5 ON CONFLICT DO NOTHING;

-- scene B1
DELETE FROM build_queue WHERE settlement_id='54b1a5b5-1d02-48b6-91be-48c5f29b510d'; INSERT INTO buildings(settlement_id,building_type,level) VALUES('54b1a5b5-1d02-48b6-91be-48c5f29b510d','shipyard',2),('54b1a5b5-1d02-48b6-91be-48c5f29b510d','workshop',2) ON CONFLICT DO NOTHING;

-- scene B2
UPDATE settlement_goods SET rate=0,calc_tick=100,amount=CASE good_key WHEN 'grain' THEN 3200 WHEN 'fish' THEN 800 WHEN 'oil' THEN 150 WHEN 'timber' THEN 340 WHEN 'silver' THEN 1200 WHEN 'copper' THEN 120 WHEN 'tin' THEN 60 WHEN 'bronze' THEN 45 WHEN 'pottery' THEN 24 WHEN 'horses' THEN 8 WHEN 'livestock' THEN 40 WHEN 'stone' THEN 200 WHEN 'wine' THEN 60 WHEN 'cult' THEN 0 ELSE 0 END WHERE settlement_id='54b1a5b5-1d02-48b6-91be-48c5f29b510d'; INSERT INTO buildings(settlement_id,building_type,level) VALUES('54b1a5b5-1d02-48b6-91be-48c5f29b510d','market',1),('54b1a5b5-1d02-48b6-91be-48c5f29b510d','barracks',1),('54b1a5b5-1d02-48b6-91be-48c5f29b510d','stable',1),('54b1a5b5-1d02-48b6-91be-48c5f29b510d','silo',1),('54b1a5b5-1d02-48b6-91be-48c5f29b510d','pen',1),('54b1a5b5-1d02-48b6-91be-48c5f29b510d','harbour',1) ON CONFLICT DO NOTHING; UPDATE provinces SET coastal=true WHERE id='801b0e58-a7ae-467c-8dfc-54ba46b8a07c'; INSERT INTO market_boards(settlement_id,world_id,buys,sells,written_tick) VALUES('54b1a5b5-1d02-48b6-91be-48c5f29b510d','39addc16-6709-4b1f-a609-ab68f50d7354','{tin,copper}','{pottery,oil}',90) ON CONFLICT DO NOTHING; INSERT INTO player_scouted_tiles(world_id,player_id,q,r) SELECT '39addc16-6709-4b1f-a609-ab68f50d7354','374645c6-b550-4a31-9faa-58af0e2e5950',q,r FROM map_tiles WHERE world_id='39addc16-6709-4b1f-a609-ab68f50d7354' AND (abs(q-6)+abs(q+r-6-24)+abs(r-24))/2<=5 ON CONFLICT DO NOTHING;

-- scene B2
DELETE FROM units WHERE owner_id='374645c6-b550-4a31-9faa-58af0e2e5950' AND type='galley'; INSERT INTO units(world_id,owner_id,type,category,size,crew,status,settlement_id,support_settlement_id,home_settlement_id,q,r,name,hull) VALUES('39addc16-6709-4b1f-a609-ab68f50d7354','374645c6-b550-4a31-9faa-58af0e2e5950','galley','naval',0,20,'garrison','54b1a5b5-1d02-48b6-91be-48c5f29b510d','54b1a5b5-1d02-48b6-91be-48c5f29b510d','54b1a5b5-1d02-48b6-91be-48c5f29b510d',6,24,'Dolphin',2),('39addc16-6709-4b1f-a609-ab68f50d7354','374645c6-b550-4a31-9faa-58af0e2e5950','galley','naval',0,20,'garrison','54b1a5b5-1d02-48b6-91be-48c5f29b510d','54b1a5b5-1d02-48b6-91be-48c5f29b510d','54b1a5b5-1d02-48b6-91be-48c5f29b510d',6,24,'Swallow',5);

-- scene B2
DELETE FROM player_storm_sightings WHERE storm_id IN(SELECT id FROM sea_storms WHERE world_id='39addc16-6709-4b1f-a609-ab68f50d7354'); DELETE FROM sea_storm_track WHERE storm_id IN(SELECT id FROM sea_storms WHERE world_id='39addc16-6709-4b1f-a609-ab68f50d7354'); DELETE FROM sea_storms WHERE world_id='39addc16-6709-4b1f-a609-ab68f50d7354';

-- scene B2
DELETE FROM buildings WHERE settlement_id='54b1a5b5-1d02-48b6-91be-48c5f29b510d' AND building_type='temple'; INSERT INTO buildings(settlement_id,building_type,level,god_id) VALUES('54b1a5b5-1d02-48b6-91be-48c5f29b510d','temple',3,'potnia'),('54b1a5b5-1d02-48b6-91be-48c5f29b510d','temple',3,'velchanos'),('54b1a5b5-1d02-48b6-91be-48c5f29b510d','temple',3,'asterion'),('54b1a5b5-1d02-48b6-91be-48c5f29b510d','temple',3,'sea_daimons_stillness'); UPDATE player_world_records SET kharis_amount=90,kharis_rate=0,kharis_calc_tick=100 WHERE world_id='39addc16-6709-4b1f-a609-ab68f50d7354' AND player_id='374645c6-b550-4a31-9faa-58af0e2e5950'; UPDATE settlement_goods SET amount=CASE good_key WHEN 'fish' THEN 900 WHEN 'wine' THEN 400 WHEN 'oil' THEN 500 WHEN 'livestock' THEN 300 WHEN 'silver' THEN 1200 WHEN 'timber' THEN 600 ELSE amount END WHERE settlement_id='54b1a5b5-1d02-48b6-91be-48c5f29b510d'; DELETE FROM temple_rest; DELETE FROM prayer_blessings; DELETE FROM oracle_sights;

-- scene B2
INSERT INTO notifications(world_id,player_id,kind,level,body_json,world_tick) VALUES('39addc16-6709-4b1f-a609-ab68f50d7354','374645c6-b550-4a31-9faa-58af0e2e5950','ShipStormDamaged',2,'{"name":"Dolphin","ship_type":"galley","q":8,"r":26,"hull":5,"hull_max":5,"divine":"warded","storm_name":"Notos"}',100)

-- scene B3
UPDATE settlement_goods SET rate=0,calc_tick=100,amount=CASE good_key WHEN 'grain' THEN 3200 WHEN 'fish' THEN 800 WHEN 'oil' THEN 150 WHEN 'timber' THEN 340 WHEN 'silver' THEN 1200 WHEN 'copper' THEN 120 WHEN 'tin' THEN 60 WHEN 'bronze' THEN 45 WHEN 'pottery' THEN 24 WHEN 'horses' THEN 8 WHEN 'livestock' THEN 40 WHEN 'stone' THEN 200 WHEN 'wine' THEN 60 WHEN 'cult' THEN 0 ELSE 0 END WHERE settlement_id='54b1a5b5-1d02-48b6-91be-48c5f29b510d'; INSERT INTO buildings(settlement_id,building_type,level) VALUES('54b1a5b5-1d02-48b6-91be-48c5f29b510d','market',1),('54b1a5b5-1d02-48b6-91be-48c5f29b510d','barracks',1),('54b1a5b5-1d02-48b6-91be-48c5f29b510d','stable',1),('54b1a5b5-1d02-48b6-91be-48c5f29b510d','silo',1),('54b1a5b5-1d02-48b6-91be-48c5f29b510d','pen',1),('54b1a5b5-1d02-48b6-91be-48c5f29b510d','harbour',1) ON CONFLICT DO NOTHING; UPDATE provinces SET coastal=true WHERE id='801b0e58-a7ae-467c-8dfc-54ba46b8a07c'; INSERT INTO market_boards(settlement_id,world_id,buys,sells,written_tick) VALUES('54b1a5b5-1d02-48b6-91be-48c5f29b510d','39addc16-6709-4b1f-a609-ab68f50d7354','{tin,copper}','{pottery,oil}',90) ON CONFLICT DO NOTHING; INSERT INTO player_scouted_tiles(world_id,player_id,q,r) SELECT '39addc16-6709-4b1f-a609-ab68f50d7354','374645c6-b550-4a31-9faa-58af0e2e5950',q,r FROM map_tiles WHERE world_id='39addc16-6709-4b1f-a609-ab68f50d7354' AND (abs(q-6)+abs(q+r-6-24)+abs(r-24))/2<=5 ON CONFLICT DO NOTHING;

-- scene B3
DELETE FROM units WHERE owner_id='374645c6-b550-4a31-9faa-58af0e2e5950' AND type='galley'; INSERT INTO units(world_id,owner_id,type,category,size,crew,status,settlement_id,support_settlement_id,home_settlement_id,q,r,name,hull) VALUES('39addc16-6709-4b1f-a609-ab68f50d7354','374645c6-b550-4a31-9faa-58af0e2e5950','galley','naval',0,20,'garrison','54b1a5b5-1d02-48b6-91be-48c5f29b510d','54b1a5b5-1d02-48b6-91be-48c5f29b510d','54b1a5b5-1d02-48b6-91be-48c5f29b510d',6,24,'Dolphin',2),('39addc16-6709-4b1f-a609-ab68f50d7354','374645c6-b550-4a31-9faa-58af0e2e5950','galley','naval',0,20,'garrison','54b1a5b5-1d02-48b6-91be-48c5f29b510d','54b1a5b5-1d02-48b6-91be-48c5f29b510d','54b1a5b5-1d02-48b6-91be-48c5f29b510d',6,24,'Swallow',5);

-- scene B3
DELETE FROM player_storm_sightings WHERE storm_id IN(SELECT id FROM sea_storms WHERE world_id='39addc16-6709-4b1f-a609-ab68f50d7354'); DELETE FROM sea_storm_track WHERE storm_id IN(SELECT id FROM sea_storms WHERE world_id='39addc16-6709-4b1f-a609-ab68f50d7354'); DELETE FROM sea_storms WHERE world_id='39addc16-6709-4b1f-a609-ab68f50d7354';

-- scene B3
DELETE FROM buildings WHERE settlement_id='54b1a5b5-1d02-48b6-91be-48c5f29b510d' AND building_type='temple'; INSERT INTO buildings(settlement_id,building_type,level,god_id) VALUES('54b1a5b5-1d02-48b6-91be-48c5f29b510d','temple',3,'potnia'),('54b1a5b5-1d02-48b6-91be-48c5f29b510d','temple',3,'velchanos'),('54b1a5b5-1d02-48b6-91be-48c5f29b510d','temple',3,'asterion'),('54b1a5b5-1d02-48b6-91be-48c5f29b510d','temple',3,'sea_daimons_stillness'); UPDATE player_world_records SET kharis_amount=90,kharis_rate=0,kharis_calc_tick=100 WHERE world_id='39addc16-6709-4b1f-a609-ab68f50d7354' AND player_id='374645c6-b550-4a31-9faa-58af0e2e5950'; UPDATE settlement_goods SET amount=CASE good_key WHEN 'fish' THEN 900 WHEN 'wine' THEN 400 WHEN 'oil' THEN 500 WHEN 'livestock' THEN 300 WHEN 'silver' THEN 1200 WHEN 'timber' THEN 600 ELSE amount END WHERE settlement_id='54b1a5b5-1d02-48b6-91be-48c5f29b510d'; DELETE FROM temple_rest; DELETE FROM prayer_blessings; DELETE FROM oracle_sights;

-- scene B4
UPDATE settlement_goods SET rate=0,calc_tick=100,amount=CASE good_key WHEN 'grain' THEN 3200 WHEN 'fish' THEN 800 WHEN 'oil' THEN 150 WHEN 'timber' THEN 340 WHEN 'silver' THEN 1200 WHEN 'copper' THEN 120 WHEN 'tin' THEN 60 WHEN 'bronze' THEN 45 WHEN 'pottery' THEN 24 WHEN 'horses' THEN 8 WHEN 'livestock' THEN 40 WHEN 'stone' THEN 200 WHEN 'wine' THEN 60 WHEN 'cult' THEN 0 ELSE 0 END WHERE settlement_id='54b1a5b5-1d02-48b6-91be-48c5f29b510d'; INSERT INTO buildings(settlement_id,building_type,level) VALUES('54b1a5b5-1d02-48b6-91be-48c5f29b510d','market',1),('54b1a5b5-1d02-48b6-91be-48c5f29b510d','barracks',1),('54b1a5b5-1d02-48b6-91be-48c5f29b510d','stable',1),('54b1a5b5-1d02-48b6-91be-48c5f29b510d','silo',1),('54b1a5b5-1d02-48b6-91be-48c5f29b510d','pen',1),('54b1a5b5-1d02-48b6-91be-48c5f29b510d','harbour',1) ON CONFLICT DO NOTHING; UPDATE provinces SET coastal=true WHERE id='801b0e58-a7ae-467c-8dfc-54ba46b8a07c'; INSERT INTO market_boards(settlement_id,world_id,buys,sells,written_tick) VALUES('54b1a5b5-1d02-48b6-91be-48c5f29b510d','39addc16-6709-4b1f-a609-ab68f50d7354','{tin,copper}','{pottery,oil}',90) ON CONFLICT DO NOTHING; INSERT INTO player_scouted_tiles(world_id,player_id,q,r) SELECT '39addc16-6709-4b1f-a609-ab68f50d7354','374645c6-b550-4a31-9faa-58af0e2e5950',q,r FROM map_tiles WHERE world_id='39addc16-6709-4b1f-a609-ab68f50d7354' AND (abs(q-6)+abs(q+r-6-24)+abs(r-24))/2<=5 ON CONFLICT DO NOTHING;

-- scene B4
DELETE FROM units WHERE owner_id='374645c6-b550-4a31-9faa-58af0e2e5950' AND type='galley'; INSERT INTO units(world_id,owner_id,type,category,size,crew,status,settlement_id,support_settlement_id,home_settlement_id,q,r,name,hull) VALUES('39addc16-6709-4b1f-a609-ab68f50d7354','374645c6-b550-4a31-9faa-58af0e2e5950','galley','naval',0,20,'garrison','54b1a5b5-1d02-48b6-91be-48c5f29b510d','54b1a5b5-1d02-48b6-91be-48c5f29b510d','54b1a5b5-1d02-48b6-91be-48c5f29b510d',6,24,'Dolphin',2),('39addc16-6709-4b1f-a609-ab68f50d7354','374645c6-b550-4a31-9faa-58af0e2e5950','galley','naval',0,20,'garrison','54b1a5b5-1d02-48b6-91be-48c5f29b510d','54b1a5b5-1d02-48b6-91be-48c5f29b510d','54b1a5b5-1d02-48b6-91be-48c5f29b510d',6,24,'Swallow',5);

-- scene B4
DELETE FROM player_storm_sightings WHERE storm_id IN(SELECT id FROM sea_storms WHERE world_id='39addc16-6709-4b1f-a609-ab68f50d7354'); DELETE FROM sea_storm_track WHERE storm_id IN(SELECT id FROM sea_storms WHERE world_id='39addc16-6709-4b1f-a609-ab68f50d7354'); DELETE FROM sea_storms WHERE world_id='39addc16-6709-4b1f-a609-ab68f50d7354';

-- scene B4
DELETE FROM buildings WHERE settlement_id='54b1a5b5-1d02-48b6-91be-48c5f29b510d' AND building_type='temple'; INSERT INTO buildings(settlement_id,building_type,level,god_id) VALUES('54b1a5b5-1d02-48b6-91be-48c5f29b510d','temple',3,'potnia'),('54b1a5b5-1d02-48b6-91be-48c5f29b510d','temple',3,'velchanos'),('54b1a5b5-1d02-48b6-91be-48c5f29b510d','temple',3,'asterion'),('54b1a5b5-1d02-48b6-91be-48c5f29b510d','temple',3,'sea_daimons_stillness'); UPDATE player_world_records SET kharis_amount=90,kharis_rate=0,kharis_calc_tick=100 WHERE world_id='39addc16-6709-4b1f-a609-ab68f50d7354' AND player_id='374645c6-b550-4a31-9faa-58af0e2e5950'; UPDATE settlement_goods SET amount=CASE good_key WHEN 'fish' THEN 900 WHEN 'wine' THEN 400 WHEN 'oil' THEN 500 WHEN 'livestock' THEN 300 WHEN 'silver' THEN 1200 WHEN 'timber' THEN 600 ELSE amount END WHERE settlement_id='54b1a5b5-1d02-48b6-91be-48c5f29b510d'; DELETE FROM temple_rest; DELETE FROM prayer_blessings; DELETE FROM oracle_sights;

-- scene B4
DELETE FROM player_storm_sightings WHERE storm_id IN(SELECT id FROM sea_storms WHERE world_id='39addc16-6709-4b1f-a609-ab68f50d7354'); DELETE FROM sea_storm_track WHERE storm_id IN(SELECT id FROM sea_storms WHERE world_id='39addc16-6709-4b1f-a609-ab68f50d7354'); DELETE FROM sea_storms WHERE world_id='39addc16-6709-4b1f-a609-ab68f50d7354';

-- scene B4
INSERT INTO sea_storms(world_id,heading,created_tick,dies_tick,name,claimed_by,claimed_tick,named_tick) VALUES('39addc16-6709-4b1f-a609-ab68f50d7354',2,60,100000,'Notos','374645c6-b550-4a31-9faa-58af0e2e5950',70,70) RETURNING id

-- scene B4
INSERT INTO sea_storm_track(storm_id,tick,slot,q,r) VALUES ('533d290b-369b-44b0-b778-4032dfa1e722',100,0,7,26),('533d290b-369b-44b0-b778-4032dfa1e722',100,1,8,26)

-- scene B4
INSERT INTO player_storm_sightings(player_id,storm_id,seen_tick,hexes) VALUES('374645c6-b550-4a31-9faa-58af0e2e5950','533d290b-369b-44b0-b778-4032dfa1e722',100,'[{"q": 7, "r": 26}, {"q": 8, "r": 26}]')

-- scene B4
INSERT INTO sea_storms(world_id,heading,created_tick,dies_tick,name,claimed_by,claimed_tick,named_tick) VALUES('39addc16-6709-4b1f-a609-ab68f50d7354',2,60,100000,'Kaikias','374645c6-b550-4a31-9faa-58af0e2e5950',70,70) RETURNING id

-- scene B4
INSERT INTO sea_storm_track(storm_id,tick,slot,q,r) VALUES ('90ee6c4d-f80c-4825-b9aa-f86f73dc23d7',100,0,4,26)

-- scene B4
INSERT INTO player_storm_sightings(player_id,storm_id,seen_tick,hexes) VALUES('374645c6-b550-4a31-9faa-58af0e2e5950','90ee6c4d-f80c-4825-b9aa-f86f73dc23d7',100,'[{"q": 4, "r": 26}]')

-- scene B4
INSERT INTO sea_storms(world_id,heading,created_tick,dies_tick,name,claimed_by,claimed_tick,named_tick,died_tick) VALUES('39addc16-6709-4b1f-a609-ab68f50d7354',2,60,95,'Zephyros','374645c6-b550-4a31-9faa-58af0e2e5950',70,70,95) RETURNING id

-- scene B4
INSERT INTO sea_storm_track(storm_id,tick,slot,q,r) VALUES ('02079078-b20f-425a-bfb0-3272e540f7a3',100,0,14,26),('02079078-b20f-425a-bfb0-3272e540f7a3',100,1,15,26),('02079078-b20f-425a-bfb0-3272e540f7a3',100,2,14,27)

-- scene B4
INSERT INTO player_storm_sightings(player_id,storm_id,seen_tick,hexes) VALUES('374645c6-b550-4a31-9faa-58af0e2e5950','02079078-b20f-425a-bfb0-3272e540f7a3',90,'[{"q": 14, "r": 26}, {"q": 15, "r": 26}, {"q": 14, "r": 27}]')

-- scene B5
UPDATE settlement_goods SET rate=0,calc_tick=100,amount=CASE good_key WHEN 'grain' THEN 3200 WHEN 'fish' THEN 800 WHEN 'oil' THEN 150 WHEN 'timber' THEN 340 WHEN 'silver' THEN 1200 WHEN 'copper' THEN 120 WHEN 'tin' THEN 60 WHEN 'bronze' THEN 45 WHEN 'pottery' THEN 24 WHEN 'horses' THEN 8 WHEN 'livestock' THEN 40 WHEN 'stone' THEN 200 WHEN 'wine' THEN 60 WHEN 'cult' THEN 0 ELSE 0 END WHERE settlement_id='54b1a5b5-1d02-48b6-91be-48c5f29b510d'; INSERT INTO buildings(settlement_id,building_type,level) VALUES('54b1a5b5-1d02-48b6-91be-48c5f29b510d','market',1),('54b1a5b5-1d02-48b6-91be-48c5f29b510d','barracks',1),('54b1a5b5-1d02-48b6-91be-48c5f29b510d','stable',1),('54b1a5b5-1d02-48b6-91be-48c5f29b510d','silo',1),('54b1a5b5-1d02-48b6-91be-48c5f29b510d','pen',1),('54b1a5b5-1d02-48b6-91be-48c5f29b510d','harbour',1) ON CONFLICT DO NOTHING; UPDATE provinces SET coastal=true WHERE id='801b0e58-a7ae-467c-8dfc-54ba46b8a07c'; INSERT INTO market_boards(settlement_id,world_id,buys,sells,written_tick) VALUES('54b1a5b5-1d02-48b6-91be-48c5f29b510d','39addc16-6709-4b1f-a609-ab68f50d7354','{tin,copper}','{pottery,oil}',90) ON CONFLICT DO NOTHING; INSERT INTO player_scouted_tiles(world_id,player_id,q,r) SELECT '39addc16-6709-4b1f-a609-ab68f50d7354','374645c6-b550-4a31-9faa-58af0e2e5950',q,r FROM map_tiles WHERE world_id='39addc16-6709-4b1f-a609-ab68f50d7354' AND (abs(q-6)+abs(q+r-6-24)+abs(r-24))/2<=5 ON CONFLICT DO NOTHING;

-- scene B5
DELETE FROM units WHERE owner_id='374645c6-b550-4a31-9faa-58af0e2e5950' AND type='galley'; INSERT INTO units(world_id,owner_id,type,category,size,crew,status,settlement_id,support_settlement_id,home_settlement_id,q,r,name,hull) VALUES('39addc16-6709-4b1f-a609-ab68f50d7354','374645c6-b550-4a31-9faa-58af0e2e5950','galley','naval',0,20,'garrison','54b1a5b5-1d02-48b6-91be-48c5f29b510d','54b1a5b5-1d02-48b6-91be-48c5f29b510d','54b1a5b5-1d02-48b6-91be-48c5f29b510d',6,24,'Dolphin',2),('39addc16-6709-4b1f-a609-ab68f50d7354','374645c6-b550-4a31-9faa-58af0e2e5950','galley','naval',0,20,'garrison','54b1a5b5-1d02-48b6-91be-48c5f29b510d','54b1a5b5-1d02-48b6-91be-48c5f29b510d','54b1a5b5-1d02-48b6-91be-48c5f29b510d',6,24,'Swallow',5);

-- scene B5
DELETE FROM player_storm_sightings WHERE storm_id IN(SELECT id FROM sea_storms WHERE world_id='39addc16-6709-4b1f-a609-ab68f50d7354'); DELETE FROM sea_storm_track WHERE storm_id IN(SELECT id FROM sea_storms WHERE world_id='39addc16-6709-4b1f-a609-ab68f50d7354'); DELETE FROM sea_storms WHERE world_id='39addc16-6709-4b1f-a609-ab68f50d7354';

-- scene B5
DELETE FROM buildings WHERE settlement_id='54b1a5b5-1d02-48b6-91be-48c5f29b510d' AND building_type='temple'; INSERT INTO buildings(settlement_id,building_type,level,god_id) VALUES('54b1a5b5-1d02-48b6-91be-48c5f29b510d','temple',3,'potnia'),('54b1a5b5-1d02-48b6-91be-48c5f29b510d','temple',3,'velchanos'),('54b1a5b5-1d02-48b6-91be-48c5f29b510d','temple',3,'asterion'),('54b1a5b5-1d02-48b6-91be-48c5f29b510d','temple',3,'sea_daimons_stillness'); UPDATE player_world_records SET kharis_amount=90,kharis_rate=0,kharis_calc_tick=100 WHERE world_id='39addc16-6709-4b1f-a609-ab68f50d7354' AND player_id='374645c6-b550-4a31-9faa-58af0e2e5950'; UPDATE settlement_goods SET amount=CASE good_key WHEN 'fish' THEN 900 WHEN 'wine' THEN 400 WHEN 'oil' THEN 500 WHEN 'livestock' THEN 300 WHEN 'silver' THEN 1200 WHEN 'timber' THEN 600 ELSE amount END WHERE settlement_id='54b1a5b5-1d02-48b6-91be-48c5f29b510d'; DELETE FROM temple_rest; DELETE FROM prayer_blessings; DELETE FROM oracle_sights;

-- scene B5
DELETE FROM player_storm_sightings WHERE storm_id IN(SELECT id FROM sea_storms WHERE world_id='39addc16-6709-4b1f-a609-ab68f50d7354'); DELETE FROM sea_storm_track WHERE storm_id IN(SELECT id FROM sea_storms WHERE world_id='39addc16-6709-4b1f-a609-ab68f50d7354'); DELETE FROM sea_storms WHERE world_id='39addc16-6709-4b1f-a609-ab68f50d7354';

-- scene B5
INSERT INTO sea_storms(world_id,heading,created_tick,dies_tick,name,claimed_by,claimed_tick,named_tick) VALUES('39addc16-6709-4b1f-a609-ab68f50d7354',2,60,100000,'Notos','374645c6-b550-4a31-9faa-58af0e2e5950',70,70) RETURNING id

-- scene B5
INSERT INTO sea_storm_track(storm_id,tick,slot,q,r) VALUES ('b434006f-d086-4b92-a8dc-cd3a8dc37ea7',100,0,7,26),('b434006f-d086-4b92-a8dc-cd3a8dc37ea7',100,1,8,26)

-- scene B5
INSERT INTO player_storm_sightings(player_id,storm_id,seen_tick,hexes) VALUES('374645c6-b550-4a31-9faa-58af0e2e5950','b434006f-d086-4b92-a8dc-cd3a8dc37ea7',100,'[{"q": 7, "r": 26}, {"q": 8, "r": 26}]')

-- scene B5
INSERT INTO sea_storms(world_id,heading,created_tick,dies_tick,name,claimed_by,claimed_tick,named_tick) VALUES('39addc16-6709-4b1f-a609-ab68f50d7354',2,60,100000,'Kaikias','374645c6-b550-4a31-9faa-58af0e2e5950',70,70) RETURNING id

-- scene B5
INSERT INTO sea_storm_track(storm_id,tick,slot,q,r) VALUES ('5059ebfa-8d29-4c60-932b-b4d5f53ee8d2',100,0,4,26)

-- scene B5
INSERT INTO player_storm_sightings(player_id,storm_id,seen_tick,hexes) VALUES('374645c6-b550-4a31-9faa-58af0e2e5950','5059ebfa-8d29-4c60-932b-b4d5f53ee8d2',100,'[{"q": 4, "r": 26}]')

-- scene B6
UPDATE settlement_goods SET rate=0,calc_tick=100,amount=CASE good_key WHEN 'grain' THEN 3200 WHEN 'fish' THEN 800 WHEN 'oil' THEN 150 WHEN 'timber' THEN 340 WHEN 'silver' THEN 1200 WHEN 'copper' THEN 120 WHEN 'tin' THEN 60 WHEN 'bronze' THEN 45 WHEN 'pottery' THEN 24 WHEN 'horses' THEN 8 WHEN 'livestock' THEN 40 WHEN 'stone' THEN 200 WHEN 'wine' THEN 60 WHEN 'cult' THEN 0 ELSE 0 END WHERE settlement_id='54b1a5b5-1d02-48b6-91be-48c5f29b510d'; INSERT INTO buildings(settlement_id,building_type,level) VALUES('54b1a5b5-1d02-48b6-91be-48c5f29b510d','market',1),('54b1a5b5-1d02-48b6-91be-48c5f29b510d','barracks',1),('54b1a5b5-1d02-48b6-91be-48c5f29b510d','stable',1),('54b1a5b5-1d02-48b6-91be-48c5f29b510d','silo',1),('54b1a5b5-1d02-48b6-91be-48c5f29b510d','pen',1),('54b1a5b5-1d02-48b6-91be-48c5f29b510d','harbour',1) ON CONFLICT DO NOTHING; UPDATE provinces SET coastal=true WHERE id='801b0e58-a7ae-467c-8dfc-54ba46b8a07c'; INSERT INTO market_boards(settlement_id,world_id,buys,sells,written_tick) VALUES('54b1a5b5-1d02-48b6-91be-48c5f29b510d','39addc16-6709-4b1f-a609-ab68f50d7354','{tin,copper}','{pottery,oil}',90) ON CONFLICT DO NOTHING; INSERT INTO player_scouted_tiles(world_id,player_id,q,r) SELECT '39addc16-6709-4b1f-a609-ab68f50d7354','374645c6-b550-4a31-9faa-58af0e2e5950',q,r FROM map_tiles WHERE world_id='39addc16-6709-4b1f-a609-ab68f50d7354' AND (abs(q-6)+abs(q+r-6-24)+abs(r-24))/2<=5 ON CONFLICT DO NOTHING;

-- scene B6
DELETE FROM units WHERE owner_id='374645c6-b550-4a31-9faa-58af0e2e5950' AND type='galley'; INSERT INTO units(world_id,owner_id,type,category,size,crew,status,settlement_id,support_settlement_id,home_settlement_id,q,r,name,hull) VALUES('39addc16-6709-4b1f-a609-ab68f50d7354','374645c6-b550-4a31-9faa-58af0e2e5950','galley','naval',0,20,'garrison','54b1a5b5-1d02-48b6-91be-48c5f29b510d','54b1a5b5-1d02-48b6-91be-48c5f29b510d','54b1a5b5-1d02-48b6-91be-48c5f29b510d',6,24,'Dolphin',2),('39addc16-6709-4b1f-a609-ab68f50d7354','374645c6-b550-4a31-9faa-58af0e2e5950','galley','naval',0,20,'garrison','54b1a5b5-1d02-48b6-91be-48c5f29b510d','54b1a5b5-1d02-48b6-91be-48c5f29b510d','54b1a5b5-1d02-48b6-91be-48c5f29b510d',6,24,'Swallow',5);

-- scene B6
DELETE FROM player_storm_sightings WHERE storm_id IN(SELECT id FROM sea_storms WHERE world_id='39addc16-6709-4b1f-a609-ab68f50d7354'); DELETE FROM sea_storm_track WHERE storm_id IN(SELECT id FROM sea_storms WHERE world_id='39addc16-6709-4b1f-a609-ab68f50d7354'); DELETE FROM sea_storms WHERE world_id='39addc16-6709-4b1f-a609-ab68f50d7354';

-- scene B6
DELETE FROM buildings WHERE settlement_id='54b1a5b5-1d02-48b6-91be-48c5f29b510d' AND building_type='temple'; INSERT INTO buildings(settlement_id,building_type,level,god_id) VALUES('54b1a5b5-1d02-48b6-91be-48c5f29b510d','temple',3,'potnia'),('54b1a5b5-1d02-48b6-91be-48c5f29b510d','temple',3,'velchanos'),('54b1a5b5-1d02-48b6-91be-48c5f29b510d','temple',3,'asterion'),('54b1a5b5-1d02-48b6-91be-48c5f29b510d','temple',3,'sea_daimons_stillness'); UPDATE player_world_records SET kharis_amount=90,kharis_rate=0,kharis_calc_tick=100 WHERE world_id='39addc16-6709-4b1f-a609-ab68f50d7354' AND player_id='374645c6-b550-4a31-9faa-58af0e2e5950'; UPDATE settlement_goods SET amount=CASE good_key WHEN 'fish' THEN 900 WHEN 'wine' THEN 400 WHEN 'oil' THEN 500 WHEN 'livestock' THEN 300 WHEN 'silver' THEN 1200 WHEN 'timber' THEN 600 ELSE amount END WHERE settlement_id='54b1a5b5-1d02-48b6-91be-48c5f29b510d'; DELETE FROM temple_rest; DELETE FROM prayer_blessings; DELETE FROM oracle_sights;

-- scene B6
DELETE FROM units WHERE owner_id='374645c6-b550-4a31-9faa-58af0e2e5950' AND type='galley'; INSERT INTO units(world_id,owner_id,type,category,size,crew,status,settlement_id,support_settlement_id,home_settlement_id,q,r,name,hull) VALUES('39addc16-6709-4b1f-a609-ab68f50d7354','374645c6-b550-4a31-9faa-58af0e2e5950','galley','naval',0,20,'garrison','54b1a5b5-1d02-48b6-91be-48c5f29b510d','54b1a5b5-1d02-48b6-91be-48c5f29b510d','54b1a5b5-1d02-48b6-91be-48c5f29b510d',6,24,'Dolphin',5),('39addc16-6709-4b1f-a609-ab68f50d7354','374645c6-b550-4a31-9faa-58af0e2e5950','galley','naval',0,20,'garrison','54b1a5b5-1d02-48b6-91be-48c5f29b510d','54b1a5b5-1d02-48b6-91be-48c5f29b510d','54b1a5b5-1d02-48b6-91be-48c5f29b510d',6,24,'Swallow',5);

-- scene B7
UPDATE settlement_goods SET rate=0,calc_tick=100,amount=CASE good_key WHEN 'grain' THEN 3200 WHEN 'fish' THEN 800 WHEN 'oil' THEN 150 WHEN 'timber' THEN 340 WHEN 'silver' THEN 1200 WHEN 'copper' THEN 120 WHEN 'tin' THEN 60 WHEN 'bronze' THEN 45 WHEN 'pottery' THEN 24 WHEN 'horses' THEN 8 WHEN 'livestock' THEN 40 WHEN 'stone' THEN 200 WHEN 'wine' THEN 60 WHEN 'cult' THEN 0 ELSE 0 END WHERE settlement_id='54b1a5b5-1d02-48b6-91be-48c5f29b510d'; INSERT INTO buildings(settlement_id,building_type,level) VALUES('54b1a5b5-1d02-48b6-91be-48c5f29b510d','market',1),('54b1a5b5-1d02-48b6-91be-48c5f29b510d','barracks',1),('54b1a5b5-1d02-48b6-91be-48c5f29b510d','stable',1),('54b1a5b5-1d02-48b6-91be-48c5f29b510d','silo',1),('54b1a5b5-1d02-48b6-91be-48c5f29b510d','pen',1),('54b1a5b5-1d02-48b6-91be-48c5f29b510d','harbour',1) ON CONFLICT DO NOTHING; UPDATE provinces SET coastal=true WHERE id='801b0e58-a7ae-467c-8dfc-54ba46b8a07c'; INSERT INTO market_boards(settlement_id,world_id,buys,sells,written_tick) VALUES('54b1a5b5-1d02-48b6-91be-48c5f29b510d','39addc16-6709-4b1f-a609-ab68f50d7354','{tin,copper}','{pottery,oil}',90) ON CONFLICT DO NOTHING; INSERT INTO player_scouted_tiles(world_id,player_id,q,r) SELECT '39addc16-6709-4b1f-a609-ab68f50d7354','374645c6-b550-4a31-9faa-58af0e2e5950',q,r FROM map_tiles WHERE world_id='39addc16-6709-4b1f-a609-ab68f50d7354' AND (abs(q-6)+abs(q+r-6-24)+abs(r-24))/2<=5 ON CONFLICT DO NOTHING;

-- scene B7
DELETE FROM units WHERE owner_id='374645c6-b550-4a31-9faa-58af0e2e5950' AND type='galley'; INSERT INTO units(world_id,owner_id,type,category,size,crew,status,settlement_id,support_settlement_id,home_settlement_id,q,r,name,hull) VALUES('39addc16-6709-4b1f-a609-ab68f50d7354','374645c6-b550-4a31-9faa-58af0e2e5950','galley','naval',0,20,'garrison','54b1a5b5-1d02-48b6-91be-48c5f29b510d','54b1a5b5-1d02-48b6-91be-48c5f29b510d','54b1a5b5-1d02-48b6-91be-48c5f29b510d',6,24,'Dolphin',2),('39addc16-6709-4b1f-a609-ab68f50d7354','374645c6-b550-4a31-9faa-58af0e2e5950','galley','naval',0,20,'garrison','54b1a5b5-1d02-48b6-91be-48c5f29b510d','54b1a5b5-1d02-48b6-91be-48c5f29b510d','54b1a5b5-1d02-48b6-91be-48c5f29b510d',6,24,'Swallow',5);

-- scene B7
DELETE FROM player_storm_sightings WHERE storm_id IN(SELECT id FROM sea_storms WHERE world_id='39addc16-6709-4b1f-a609-ab68f50d7354'); DELETE FROM sea_storm_track WHERE storm_id IN(SELECT id FROM sea_storms WHERE world_id='39addc16-6709-4b1f-a609-ab68f50d7354'); DELETE FROM sea_storms WHERE world_id='39addc16-6709-4b1f-a609-ab68f50d7354';

-- scene B7
DELETE FROM buildings WHERE settlement_id='54b1a5b5-1d02-48b6-91be-48c5f29b510d' AND building_type='temple'; INSERT INTO buildings(settlement_id,building_type,level,god_id) VALUES('54b1a5b5-1d02-48b6-91be-48c5f29b510d','temple',3,'potnia'),('54b1a5b5-1d02-48b6-91be-48c5f29b510d','temple',3,'velchanos'),('54b1a5b5-1d02-48b6-91be-48c5f29b510d','temple',3,'asterion'),('54b1a5b5-1d02-48b6-91be-48c5f29b510d','temple',3,'sea_daimons_stillness'); UPDATE player_world_records SET kharis_amount=90,kharis_rate=0,kharis_calc_tick=100 WHERE world_id='39addc16-6709-4b1f-a609-ab68f50d7354' AND player_id='374645c6-b550-4a31-9faa-58af0e2e5950'; UPDATE settlement_goods SET amount=CASE good_key WHEN 'fish' THEN 900 WHEN 'wine' THEN 400 WHEN 'oil' THEN 500 WHEN 'livestock' THEN 300 WHEN 'silver' THEN 1200 WHEN 'timber' THEN 600 ELSE amount END WHERE settlement_id='54b1a5b5-1d02-48b6-91be-48c5f29b510d'; DELETE FROM temple_rest; DELETE FROM prayer_blessings; DELETE FROM oracle_sights;

-- scene B7
DELETE FROM build_queue WHERE settlement_id='54b1a5b5-1d02-48b6-91be-48c5f29b510d'; INSERT INTO buildings(settlement_id,building_type,level) VALUES('54b1a5b5-1d02-48b6-91be-48c5f29b510d','workshop',2) ON CONFLICT DO NOTHING;

-- scene B7
INSERT INTO notifications(world_id,player_id,kind,level,body_json,world_tick) VALUES('39addc16-6709-4b1f-a609-ab68f50d7354','374645c6-b550-4a31-9faa-58af0e2e5950','DivinePunishmentAverted',3,'{"name":"Phaistos","would_have":"chariot_loss","settlement_id":"54b1a5b5-1d02-48b6-91be-48c5f29b510d","protection_until":"2026-11-10T12:00:00+01:00"}',100)

-- scene B8
UPDATE settlement_goods SET rate=0,calc_tick=100,amount=CASE good_key WHEN 'grain' THEN 3200 WHEN 'fish' THEN 800 WHEN 'oil' THEN 150 WHEN 'timber' THEN 340 WHEN 'silver' THEN 1200 WHEN 'copper' THEN 120 WHEN 'tin' THEN 60 WHEN 'bronze' THEN 45 WHEN 'pottery' THEN 24 WHEN 'horses' THEN 8 WHEN 'livestock' THEN 40 WHEN 'stone' THEN 200 WHEN 'wine' THEN 60 WHEN 'cult' THEN 0 ELSE 0 END WHERE settlement_id='54b1a5b5-1d02-48b6-91be-48c5f29b510d'; INSERT INTO buildings(settlement_id,building_type,level) VALUES('54b1a5b5-1d02-48b6-91be-48c5f29b510d','market',1),('54b1a5b5-1d02-48b6-91be-48c5f29b510d','barracks',1),('54b1a5b5-1d02-48b6-91be-48c5f29b510d','stable',1),('54b1a5b5-1d02-48b6-91be-48c5f29b510d','silo',1),('54b1a5b5-1d02-48b6-91be-48c5f29b510d','pen',1),('54b1a5b5-1d02-48b6-91be-48c5f29b510d','harbour',1) ON CONFLICT DO NOTHING; UPDATE provinces SET coastal=true WHERE id='801b0e58-a7ae-467c-8dfc-54ba46b8a07c'; INSERT INTO market_boards(settlement_id,world_id,buys,sells,written_tick) VALUES('54b1a5b5-1d02-48b6-91be-48c5f29b510d','39addc16-6709-4b1f-a609-ab68f50d7354','{tin,copper}','{pottery,oil}',90) ON CONFLICT DO NOTHING; INSERT INTO player_scouted_tiles(world_id,player_id,q,r) SELECT '39addc16-6709-4b1f-a609-ab68f50d7354','374645c6-b550-4a31-9faa-58af0e2e5950',q,r FROM map_tiles WHERE world_id='39addc16-6709-4b1f-a609-ab68f50d7354' AND (abs(q-6)+abs(q+r-6-24)+abs(r-24))/2<=5 ON CONFLICT DO NOTHING;

-- scene B8
DELETE FROM units WHERE owner_id='374645c6-b550-4a31-9faa-58af0e2e5950' AND type='galley'; INSERT INTO units(world_id,owner_id,type,category,size,crew,status,settlement_id,support_settlement_id,home_settlement_id,q,r,name,hull) VALUES('39addc16-6709-4b1f-a609-ab68f50d7354','374645c6-b550-4a31-9faa-58af0e2e5950','galley','naval',0,20,'garrison','54b1a5b5-1d02-48b6-91be-48c5f29b510d','54b1a5b5-1d02-48b6-91be-48c5f29b510d','54b1a5b5-1d02-48b6-91be-48c5f29b510d',6,24,'Dolphin',2),('39addc16-6709-4b1f-a609-ab68f50d7354','374645c6-b550-4a31-9faa-58af0e2e5950','galley','naval',0,20,'garrison','54b1a5b5-1d02-48b6-91be-48c5f29b510d','54b1a5b5-1d02-48b6-91be-48c5f29b510d','54b1a5b5-1d02-48b6-91be-48c5f29b510d',6,24,'Swallow',5);

-- scene B8
DELETE FROM player_storm_sightings WHERE storm_id IN(SELECT id FROM sea_storms WHERE world_id='39addc16-6709-4b1f-a609-ab68f50d7354'); DELETE FROM sea_storm_track WHERE storm_id IN(SELECT id FROM sea_storms WHERE world_id='39addc16-6709-4b1f-a609-ab68f50d7354'); DELETE FROM sea_storms WHERE world_id='39addc16-6709-4b1f-a609-ab68f50d7354';

-- scene B8
DELETE FROM buildings WHERE settlement_id='54b1a5b5-1d02-48b6-91be-48c5f29b510d' AND building_type='temple'; INSERT INTO buildings(settlement_id,building_type,level,god_id) VALUES('54b1a5b5-1d02-48b6-91be-48c5f29b510d','temple',3,'potnia'),('54b1a5b5-1d02-48b6-91be-48c5f29b510d','temple',3,'velchanos'),('54b1a5b5-1d02-48b6-91be-48c5f29b510d','temple',3,'asterion'),('54b1a5b5-1d02-48b6-91be-48c5f29b510d','temple',3,'sea_daimons_stillness'); UPDATE player_world_records SET kharis_amount=90,kharis_rate=0,kharis_calc_tick=100 WHERE world_id='39addc16-6709-4b1f-a609-ab68f50d7354' AND player_id='374645c6-b550-4a31-9faa-58af0e2e5950'; UPDATE settlement_goods SET amount=CASE good_key WHEN 'fish' THEN 900 WHEN 'wine' THEN 400 WHEN 'oil' THEN 500 WHEN 'livestock' THEN 300 WHEN 'silver' THEN 1200 WHEN 'timber' THEN 600 ELSE amount END WHERE settlement_id='54b1a5b5-1d02-48b6-91be-48c5f29b510d'; DELETE FROM temple_rest; DELETE FROM prayer_blessings; DELETE FROM oracle_sights;

-- scene B8
UPDATE units SET q=10,r=24,status='positioned',settlement_id=NULL,support_settlement_id=NULL WHERE id=(SELECT id FROM units WHERE world_id='39addc16-6709-4b1f-a609-ab68f50d7354' AND owner_id='374645c6-b550-4a31-9faa-58af0e2e5950' AND type='spearman' LIMIT 1);

-- scene B8
INSERT INTO player_seen_settlements(world_id,player_id,settlement_id,q,r,name,owner_id,owner_name,is_capital,state,wall_level,population,seen_tick) VALUES('39addc16-6709-4b1f-a609-ab68f50d7354','374645c6-b550-4a31-9faa-58af0e2e5950','b756a5e3-d16f-48df-9aba-e2c749a8258f',20,24,'Pylos','1f991980-7b82-4f7a-87d7-788c983001c5','Nestor',true,'active',0,380,40) ON CONFLICT DO NOTHING; INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('39addc16-6709-4b1f-a609-ab68f50d7354','374645c6-b550-4a31-9faa-58af0e2e5950','4313402d-b13e-4b94-a162-b3e036b8d198') ON CONFLICT DO NOTHING; INSERT INTO player_scouted_tiles(world_id,player_id,q,r) SELECT '39addc16-6709-4b1f-a609-ab68f50d7354','374645c6-b550-4a31-9faa-58af0e2e5950',q,r FROM map_tiles WHERE world_id='39addc16-6709-4b1f-a609-ab68f50d7354' AND (abs(q-20)+abs(q+r-20-24)+abs(r-24))/2<=2 ON CONFLICT DO NOTHING;

-- scene B8
INSERT INTO market_boards(settlement_id,world_id,buys,sells,written_tick) VALUES('c9505416-c067-41e7-85c4-58a731372afe','39addc16-6709-4b1f-a609-ab68f50d7354','{tin,timber}','{wine,pottery}',80) ON CONFLICT DO NOTHING; INSERT INTO market_board_readings(world_id,player_id,settlement_id,buys,sells,written_tick,read_tick) VALUES('39addc16-6709-4b1f-a609-ab68f50d7354','374645c6-b550-4a31-9faa-58af0e2e5950','c9505416-c067-41e7-85c4-58a731372afe','{tin,timber}','{wine,pottery}',80,95) ON CONFLICT DO NOTHING;

-- scene B8
INSERT INTO player_seen_settlements(world_id,player_id,settlement_id,q,r,name,owner_id,owner_name,is_capital,state,wall_level,population,seen_tick) VALUES('39addc16-6709-4b1f-a609-ab68f50d7354','374645c6-b550-4a31-9faa-58af0e2e5950','c46b9ce8-d27a-419b-b770-333058267c44',28,24,'Kydonia','0c874aab-3a7f-4381-907e-694730389f79','Atreus',true,'active',0,1500,70) ON CONFLICT DO NOTHING; INSERT INTO player_contacted_settlements(world_id,player_id,settlement_id,contact_tick,source,name,owner_id,owner_name,size,food,goods,buildings) VALUES('39addc16-6709-4b1f-a609-ab68f50d7354','374645c6-b550-4a31-9faa-58af0e2e5950','c46b9ce8-d27a-419b-b770-333058267c44',70,'messenger','Kydonia','0c874aab-3a7f-4381-907e-694730389f79','Atreus','town','well fed','[{"word":"plenty of","good":"olive oil"},{"word":"some","good":"timber"}]','[{"type":"granary","level":2},{"type":"harbour","level":1}]') ON CONFLICT DO NOTHING; INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('39addc16-6709-4b1f-a609-ab68f50d7354','374645c6-b550-4a31-9faa-58af0e2e5950','d340566a-6af1-4b93-aede-cb045908221b') ON CONFLICT DO NOTHING; INSERT INTO player_scouted_tiles(world_id,player_id,q,r) SELECT '39addc16-6709-4b1f-a609-ab68f50d7354','374645c6-b550-4a31-9faa-58af0e2e5950',q,r FROM map_tiles WHERE world_id='39addc16-6709-4b1f-a609-ab68f50d7354' AND (abs(q-28)+abs(q+r-28-24)+abs(r-24))/2<=1 ON CONFLICT DO NOTHING;

-- scene B8
UPDATE settlement_goods SET amount=10000,rate=0,cap=1000000,calc_tick=100 WHERE settlement_id='c9505416-c067-41e7-85c4-58a731372afe'

-- scene B8
UPDATE scheduled_events SET due_tick=100,process_after=now() WHERE world_id='39addc16-6709-4b1f-a609-ab68f50d7354' AND event_type LIKE 'MessengerArrival%' AND payload->>'messenger_id'='bbbd0f77-e8e6-40b7-adce-bf6701f5f1e4';

-- scene B9
UPDATE settlement_goods SET rate=0,calc_tick=100,amount=CASE good_key WHEN 'grain' THEN 3200 WHEN 'fish' THEN 800 WHEN 'oil' THEN 150 WHEN 'timber' THEN 340 WHEN 'silver' THEN 1200 WHEN 'copper' THEN 120 WHEN 'tin' THEN 60 WHEN 'bronze' THEN 45 WHEN 'pottery' THEN 24 WHEN 'horses' THEN 8 WHEN 'livestock' THEN 40 WHEN 'stone' THEN 200 WHEN 'wine' THEN 60 WHEN 'cult' THEN 0 ELSE 0 END WHERE settlement_id='54b1a5b5-1d02-48b6-91be-48c5f29b510d'; INSERT INTO buildings(settlement_id,building_type,level) VALUES('54b1a5b5-1d02-48b6-91be-48c5f29b510d','market',1),('54b1a5b5-1d02-48b6-91be-48c5f29b510d','barracks',1),('54b1a5b5-1d02-48b6-91be-48c5f29b510d','stable',1),('54b1a5b5-1d02-48b6-91be-48c5f29b510d','silo',1),('54b1a5b5-1d02-48b6-91be-48c5f29b510d','pen',1),('54b1a5b5-1d02-48b6-91be-48c5f29b510d','harbour',1) ON CONFLICT DO NOTHING; UPDATE provinces SET coastal=true WHERE id='801b0e58-a7ae-467c-8dfc-54ba46b8a07c'; INSERT INTO market_boards(settlement_id,world_id,buys,sells,written_tick) VALUES('54b1a5b5-1d02-48b6-91be-48c5f29b510d','39addc16-6709-4b1f-a609-ab68f50d7354','{tin,copper}','{pottery,oil}',90) ON CONFLICT DO NOTHING; INSERT INTO player_scouted_tiles(world_id,player_id,q,r) SELECT '39addc16-6709-4b1f-a609-ab68f50d7354','374645c6-b550-4a31-9faa-58af0e2e5950',q,r FROM map_tiles WHERE world_id='39addc16-6709-4b1f-a609-ab68f50d7354' AND (abs(q-6)+abs(q+r-6-24)+abs(r-24))/2<=5 ON CONFLICT DO NOTHING;

-- scene B9
DELETE FROM units WHERE owner_id='374645c6-b550-4a31-9faa-58af0e2e5950' AND type='galley'; INSERT INTO units(world_id,owner_id,type,category,size,crew,status,settlement_id,support_settlement_id,home_settlement_id,q,r,name,hull) VALUES('39addc16-6709-4b1f-a609-ab68f50d7354','374645c6-b550-4a31-9faa-58af0e2e5950','galley','naval',0,20,'garrison','54b1a5b5-1d02-48b6-91be-48c5f29b510d','54b1a5b5-1d02-48b6-91be-48c5f29b510d','54b1a5b5-1d02-48b6-91be-48c5f29b510d',6,24,'Dolphin',2),('39addc16-6709-4b1f-a609-ab68f50d7354','374645c6-b550-4a31-9faa-58af0e2e5950','galley','naval',0,20,'garrison','54b1a5b5-1d02-48b6-91be-48c5f29b510d','54b1a5b5-1d02-48b6-91be-48c5f29b510d','54b1a5b5-1d02-48b6-91be-48c5f29b510d',6,24,'Swallow',5);

-- scene B9
DELETE FROM player_storm_sightings WHERE storm_id IN(SELECT id FROM sea_storms WHERE world_id='39addc16-6709-4b1f-a609-ab68f50d7354'); DELETE FROM sea_storm_track WHERE storm_id IN(SELECT id FROM sea_storms WHERE world_id='39addc16-6709-4b1f-a609-ab68f50d7354'); DELETE FROM sea_storms WHERE world_id='39addc16-6709-4b1f-a609-ab68f50d7354';

-- scene B9
DELETE FROM buildings WHERE settlement_id='54b1a5b5-1d02-48b6-91be-48c5f29b510d' AND building_type='temple'; INSERT INTO buildings(settlement_id,building_type,level,god_id) VALUES('54b1a5b5-1d02-48b6-91be-48c5f29b510d','temple',3,'potnia'),('54b1a5b5-1d02-48b6-91be-48c5f29b510d','temple',3,'velchanos'),('54b1a5b5-1d02-48b6-91be-48c5f29b510d','temple',3,'asterion'),('54b1a5b5-1d02-48b6-91be-48c5f29b510d','temple',3,'sea_daimons_stillness'); UPDATE player_world_records SET kharis_amount=90,kharis_rate=0,kharis_calc_tick=100 WHERE world_id='39addc16-6709-4b1f-a609-ab68f50d7354' AND player_id='374645c6-b550-4a31-9faa-58af0e2e5950'; UPDATE settlement_goods SET amount=CASE good_key WHEN 'fish' THEN 900 WHEN 'wine' THEN 400 WHEN 'oil' THEN 500 WHEN 'livestock' THEN 300 WHEN 'silver' THEN 1200 WHEN 'timber' THEN 600 ELSE amount END WHERE settlement_id='54b1a5b5-1d02-48b6-91be-48c5f29b510d'; DELETE FROM temple_rest; DELETE FROM prayer_blessings; DELETE FROM oracle_sights;

-- scene B9
UPDATE units SET q=10,r=24,status='positioned',settlement_id=NULL,support_settlement_id=NULL WHERE id=(SELECT id FROM units WHERE world_id='39addc16-6709-4b1f-a609-ab68f50d7354' AND owner_id='374645c6-b550-4a31-9faa-58af0e2e5950' AND type='spearman' LIMIT 1);

-- scene B9
INSERT INTO player_seen_settlements(world_id,player_id,settlement_id,q,r,name,owner_id,owner_name,is_capital,state,wall_level,population,seen_tick) VALUES('39addc16-6709-4b1f-a609-ab68f50d7354','374645c6-b550-4a31-9faa-58af0e2e5950','b756a5e3-d16f-48df-9aba-e2c749a8258f',20,24,'Pylos','1f991980-7b82-4f7a-87d7-788c983001c5','Nestor',true,'active',0,380,40) ON CONFLICT DO NOTHING; INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('39addc16-6709-4b1f-a609-ab68f50d7354','374645c6-b550-4a31-9faa-58af0e2e5950','4313402d-b13e-4b94-a162-b3e036b8d198') ON CONFLICT DO NOTHING; INSERT INTO player_scouted_tiles(world_id,player_id,q,r) SELECT '39addc16-6709-4b1f-a609-ab68f50d7354','374645c6-b550-4a31-9faa-58af0e2e5950',q,r FROM map_tiles WHERE world_id='39addc16-6709-4b1f-a609-ab68f50d7354' AND (abs(q-20)+abs(q+r-20-24)+abs(r-24))/2<=2 ON CONFLICT DO NOTHING;

-- scene B9
INSERT INTO market_boards(settlement_id,world_id,buys,sells,written_tick) VALUES('c9505416-c067-41e7-85c4-58a731372afe','39addc16-6709-4b1f-a609-ab68f50d7354','{tin,timber}','{wine,pottery}',80) ON CONFLICT DO NOTHING; INSERT INTO market_board_readings(world_id,player_id,settlement_id,buys,sells,written_tick,read_tick) VALUES('39addc16-6709-4b1f-a609-ab68f50d7354','374645c6-b550-4a31-9faa-58af0e2e5950','c9505416-c067-41e7-85c4-58a731372afe','{tin,timber}','{wine,pottery}',80,95) ON CONFLICT DO NOTHING;

-- scene B9
INSERT INTO player_seen_settlements(world_id,player_id,settlement_id,q,r,name,owner_id,owner_name,is_capital,state,wall_level,population,seen_tick) VALUES('39addc16-6709-4b1f-a609-ab68f50d7354','374645c6-b550-4a31-9faa-58af0e2e5950','c46b9ce8-d27a-419b-b770-333058267c44',28,24,'Kydonia','0c874aab-3a7f-4381-907e-694730389f79','Atreus',true,'active',0,1500,70) ON CONFLICT DO NOTHING; INSERT INTO player_contacted_settlements(world_id,player_id,settlement_id,contact_tick,source,name,owner_id,owner_name,size,food,goods,buildings) VALUES('39addc16-6709-4b1f-a609-ab68f50d7354','374645c6-b550-4a31-9faa-58af0e2e5950','c46b9ce8-d27a-419b-b770-333058267c44',70,'messenger','Kydonia','0c874aab-3a7f-4381-907e-694730389f79','Atreus','town','well fed','[{"word":"plenty of","good":"olive oil"},{"word":"some","good":"timber"}]','[{"type":"granary","level":2},{"type":"harbour","level":1}]') ON CONFLICT DO NOTHING; INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('39addc16-6709-4b1f-a609-ab68f50d7354','374645c6-b550-4a31-9faa-58af0e2e5950','d340566a-6af1-4b93-aede-cb045908221b') ON CONFLICT DO NOTHING; INSERT INTO player_scouted_tiles(world_id,player_id,q,r) SELECT '39addc16-6709-4b1f-a609-ab68f50d7354','374645c6-b550-4a31-9faa-58af0e2e5950',q,r FROM map_tiles WHERE world_id='39addc16-6709-4b1f-a609-ab68f50d7354' AND (abs(q-28)+abs(q+r-28-24)+abs(r-24))/2<=1 ON CONFLICT DO NOTHING;

-- scene B9
UPDATE settlements SET wall_level=2 WHERE id='b756a5e3-d16f-48df-9aba-e2c749a8258f'; INSERT INTO buildings(settlement_id,building_type,level) VALUES('b756a5e3-d16f-48df-9aba-e2c749a8258f','wall',2) ON CONFLICT DO NOTHING; UPDATE buildings SET level=2 WHERE settlement_id='b756a5e3-d16f-48df-9aba-e2c749a8258f' AND building_type='wall'; UPDATE player_seen_settlements SET wall_level=2 WHERE player_id='374645c6-b550-4a31-9faa-58af0e2e5950' AND settlement_id='b756a5e3-d16f-48df-9aba-e2c749a8258f'; UPDATE player_world_records SET kharis_amount=80,kharis_rate=0,kharis_calc_tick=100 WHERE world_id='39addc16-6709-4b1f-a609-ab68f50d7354' AND player_id='1f991980-7b82-4f7a-87d7-788c983001c5';

-- scene B9
UPDATE scheduled_events SET due_tick=100,process_after=now() WHERE world_id='39addc16-6709-4b1f-a609-ab68f50d7354' AND event_type ILIKE '%wallsfall%'

-- scene B10
UPDATE settlement_goods SET rate=0,calc_tick=100,amount=CASE good_key WHEN 'grain' THEN 3200 WHEN 'fish' THEN 800 WHEN 'oil' THEN 150 WHEN 'timber' THEN 340 WHEN 'silver' THEN 1200 WHEN 'copper' THEN 120 WHEN 'tin' THEN 60 WHEN 'bronze' THEN 45 WHEN 'pottery' THEN 24 WHEN 'horses' THEN 8 WHEN 'livestock' THEN 40 WHEN 'stone' THEN 200 WHEN 'wine' THEN 60 WHEN 'cult' THEN 0 ELSE 0 END WHERE settlement_id='54b1a5b5-1d02-48b6-91be-48c5f29b510d'; INSERT INTO buildings(settlement_id,building_type,level) VALUES('54b1a5b5-1d02-48b6-91be-48c5f29b510d','market',1),('54b1a5b5-1d02-48b6-91be-48c5f29b510d','barracks',1),('54b1a5b5-1d02-48b6-91be-48c5f29b510d','stable',1),('54b1a5b5-1d02-48b6-91be-48c5f29b510d','silo',1),('54b1a5b5-1d02-48b6-91be-48c5f29b510d','pen',1),('54b1a5b5-1d02-48b6-91be-48c5f29b510d','harbour',1) ON CONFLICT DO NOTHING; UPDATE provinces SET coastal=true WHERE id='801b0e58-a7ae-467c-8dfc-54ba46b8a07c'; INSERT INTO market_boards(settlement_id,world_id,buys,sells,written_tick) VALUES('54b1a5b5-1d02-48b6-91be-48c5f29b510d','39addc16-6709-4b1f-a609-ab68f50d7354','{tin,copper}','{pottery,oil}',90) ON CONFLICT DO NOTHING; INSERT INTO player_scouted_tiles(world_id,player_id,q,r) SELECT '39addc16-6709-4b1f-a609-ab68f50d7354','374645c6-b550-4a31-9faa-58af0e2e5950',q,r FROM map_tiles WHERE world_id='39addc16-6709-4b1f-a609-ab68f50d7354' AND (abs(q-6)+abs(q+r-6-24)+abs(r-24))/2<=5 ON CONFLICT DO NOTHING;

-- scene B10
DELETE FROM units WHERE owner_id='374645c6-b550-4a31-9faa-58af0e2e5950' AND type='galley'; INSERT INTO units(world_id,owner_id,type,category,size,crew,status,settlement_id,support_settlement_id,home_settlement_id,q,r,name,hull) VALUES('39addc16-6709-4b1f-a609-ab68f50d7354','374645c6-b550-4a31-9faa-58af0e2e5950','galley','naval',0,20,'garrison','54b1a5b5-1d02-48b6-91be-48c5f29b510d','54b1a5b5-1d02-48b6-91be-48c5f29b510d','54b1a5b5-1d02-48b6-91be-48c5f29b510d',6,24,'Dolphin',2),('39addc16-6709-4b1f-a609-ab68f50d7354','374645c6-b550-4a31-9faa-58af0e2e5950','galley','naval',0,20,'garrison','54b1a5b5-1d02-48b6-91be-48c5f29b510d','54b1a5b5-1d02-48b6-91be-48c5f29b510d','54b1a5b5-1d02-48b6-91be-48c5f29b510d',6,24,'Swallow',5);

-- scene B10
DELETE FROM player_storm_sightings WHERE storm_id IN(SELECT id FROM sea_storms WHERE world_id='39addc16-6709-4b1f-a609-ab68f50d7354'); DELETE FROM sea_storm_track WHERE storm_id IN(SELECT id FROM sea_storms WHERE world_id='39addc16-6709-4b1f-a609-ab68f50d7354'); DELETE FROM sea_storms WHERE world_id='39addc16-6709-4b1f-a609-ab68f50d7354';

-- scene B10
DELETE FROM buildings WHERE settlement_id='54b1a5b5-1d02-48b6-91be-48c5f29b510d' AND building_type='temple'; INSERT INTO buildings(settlement_id,building_type,level,god_id) VALUES('54b1a5b5-1d02-48b6-91be-48c5f29b510d','temple',3,'potnia'),('54b1a5b5-1d02-48b6-91be-48c5f29b510d','temple',3,'velchanos'),('54b1a5b5-1d02-48b6-91be-48c5f29b510d','temple',3,'asterion'),('54b1a5b5-1d02-48b6-91be-48c5f29b510d','temple',3,'sea_daimons_stillness'); UPDATE player_world_records SET kharis_amount=90,kharis_rate=0,kharis_calc_tick=100 WHERE world_id='39addc16-6709-4b1f-a609-ab68f50d7354' AND player_id='374645c6-b550-4a31-9faa-58af0e2e5950'; UPDATE settlement_goods SET amount=CASE good_key WHEN 'fish' THEN 900 WHEN 'wine' THEN 400 WHEN 'oil' THEN 500 WHEN 'livestock' THEN 300 WHEN 'silver' THEN 1200 WHEN 'timber' THEN 600 ELSE amount END WHERE settlement_id='54b1a5b5-1d02-48b6-91be-48c5f29b510d'; DELETE FROM temple_rest; DELETE FROM prayer_blessings; DELETE FROM oracle_sights;

-- scene B10
UPDATE units SET q=10,r=24,status='positioned',settlement_id=NULL,support_settlement_id=NULL WHERE id=(SELECT id FROM units WHERE world_id='39addc16-6709-4b1f-a609-ab68f50d7354' AND owner_id='374645c6-b550-4a31-9faa-58af0e2e5950' AND type='spearman' LIMIT 1);

-- scene B10
INSERT INTO player_seen_settlements(world_id,player_id,settlement_id,q,r,name,owner_id,owner_name,is_capital,state,wall_level,population,seen_tick) VALUES('39addc16-6709-4b1f-a609-ab68f50d7354','374645c6-b550-4a31-9faa-58af0e2e5950','b756a5e3-d16f-48df-9aba-e2c749a8258f',20,24,'Pylos','1f991980-7b82-4f7a-87d7-788c983001c5','Nestor',true,'active',0,380,40) ON CONFLICT DO NOTHING; INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('39addc16-6709-4b1f-a609-ab68f50d7354','374645c6-b550-4a31-9faa-58af0e2e5950','4313402d-b13e-4b94-a162-b3e036b8d198') ON CONFLICT DO NOTHING; INSERT INTO player_scouted_tiles(world_id,player_id,q,r) SELECT '39addc16-6709-4b1f-a609-ab68f50d7354','374645c6-b550-4a31-9faa-58af0e2e5950',q,r FROM map_tiles WHERE world_id='39addc16-6709-4b1f-a609-ab68f50d7354' AND (abs(q-20)+abs(q+r-20-24)+abs(r-24))/2<=2 ON CONFLICT DO NOTHING;

-- scene B10
INSERT INTO market_boards(settlement_id,world_id,buys,sells,written_tick) VALUES('c9505416-c067-41e7-85c4-58a731372afe','39addc16-6709-4b1f-a609-ab68f50d7354','{tin,timber}','{wine,pottery}',80) ON CONFLICT DO NOTHING; INSERT INTO market_board_readings(world_id,player_id,settlement_id,buys,sells,written_tick,read_tick) VALUES('39addc16-6709-4b1f-a609-ab68f50d7354','374645c6-b550-4a31-9faa-58af0e2e5950','c9505416-c067-41e7-85c4-58a731372afe','{tin,timber}','{wine,pottery}',80,95) ON CONFLICT DO NOTHING;

-- scene B10
INSERT INTO player_seen_settlements(world_id,player_id,settlement_id,q,r,name,owner_id,owner_name,is_capital,state,wall_level,population,seen_tick) VALUES('39addc16-6709-4b1f-a609-ab68f50d7354','374645c6-b550-4a31-9faa-58af0e2e5950','c46b9ce8-d27a-419b-b770-333058267c44',28,24,'Kydonia','0c874aab-3a7f-4381-907e-694730389f79','Atreus',true,'active',0,1500,70) ON CONFLICT DO NOTHING; INSERT INTO player_contacted_settlements(world_id,player_id,settlement_id,contact_tick,source,name,owner_id,owner_name,size,food,goods,buildings) VALUES('39addc16-6709-4b1f-a609-ab68f50d7354','374645c6-b550-4a31-9faa-58af0e2e5950','c46b9ce8-d27a-419b-b770-333058267c44',70,'messenger','Kydonia','0c874aab-3a7f-4381-907e-694730389f79','Atreus','town','well fed','[{"word":"plenty of","good":"olive oil"},{"word":"some","good":"timber"}]','[{"type":"granary","level":2},{"type":"harbour","level":1}]') ON CONFLICT DO NOTHING; INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('39addc16-6709-4b1f-a609-ab68f50d7354','374645c6-b550-4a31-9faa-58af0e2e5950','d340566a-6af1-4b93-aede-cb045908221b') ON CONFLICT DO NOTHING; INSERT INTO player_scouted_tiles(world_id,player_id,q,r) SELECT '39addc16-6709-4b1f-a609-ab68f50d7354','374645c6-b550-4a31-9faa-58af0e2e5950',q,r FROM map_tiles WHERE world_id='39addc16-6709-4b1f-a609-ab68f50d7354' AND (abs(q-28)+abs(q+r-28-24)+abs(r-24))/2<=1 ON CONFLICT DO NOTHING;

-- scene B10
INSERT INTO place_names(world_id,kind,anchor_q,anchor_r,claimed_by,claimed_tick,name,named_tick) VALUES('39addc16-6709-4b1f-a609-ab68f50d7354','plain',6,24,'374645c6-b550-4a31-9faa-58af0e2e5950',90,'Alpheios',90) ON CONFLICT (world_id,kind,anchor_q,anchor_r) DO UPDATE SET name='Alpheios',claimed_by='374645c6-b550-4a31-9faa-58af0e2e5950'; UPDATE units SET q=21,r=24,status='positioned',settlement_id=NULL,support_settlement_id=NULL WHERE id=(SELECT id FROM units WHERE world_id='39addc16-6709-4b1f-a609-ab68f50d7354' AND owner_id='1f991980-7b82-4f7a-87d7-788c983001c5' AND type='spearman' LIMIT 1);

-- scene B10
UPDATE oracle_sights SET until_milli=from_milli+1 WHERE player_id='374645c6-b550-4a31-9faa-58af0e2e5950'


```
