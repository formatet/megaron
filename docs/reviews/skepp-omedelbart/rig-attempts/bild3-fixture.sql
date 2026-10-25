UPDATE map_tiles SET terrain='plains' WHERE world_id='8ac8c8fd-cbab-4eab-81cf-478dc9098005'; UPDATE map_tiles SET terrain='coastal_sea' WHERE world_id='8ac8c8fd-cbab-4eab-81cf-478dc9098005' AND q BETWEEN 12 AND 15 AND r=23; UPDATE units SET q=11,r=23,status='positioned' WHERE id='dd9147e1-8a6b-46b9-84cb-482e83462624';

UPDATE settlement_goods SET amount=10000,rate=0,cap=1000000 WHERE settlement_id='d8eb8456-96f0-4ba0-9df5-f3f72b6caac4'; DELETE FROM sea_storms WHERE world_id='8ac8c8fd-cbab-4eab-81cf-478dc9098005';

INSERT INTO units(world_id,owner_id,type,category,size,crew,hull,status,settlement_id,support_settlement_id,home_settlement_id,provisions,name) VALUES('8ac8c8fd-cbab-4eab-81cf-478dc9098005','9b155d4e-4c23-42fb-ab40-19245f360209','galley','naval',1,20,3,'garrison','d8eb8456-96f0-4ba0-9df5-f3f72b6caac4','d8eb8456-96f0-4ba0-9df5-f3f72b6caac4','d8eb8456-96f0-4ba0-9df5-f3f72b6caac4',0,'Sea Palace') RETURNING id;

UPDATE worlds SET current_tick=103,last_tick_at=now()-interval '22 seconds' WHERE id='8ac8c8fd-cbab-4eab-81cf-478dc9098005'; UPDATE settlement_goods SET calc_tick=103 WHERE settlement_id='d8eb8456-96f0-4ba0-9df5-f3f72b6caac4';
