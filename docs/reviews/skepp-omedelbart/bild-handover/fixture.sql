UPDATE map_tiles SET terrain='plains' WHERE world_id='530726e5-9a08-4279-8baf-aef7838dd4ed'; UPDATE map_tiles SET terrain='coastal_sea' WHERE world_id='530726e5-9a08-4279-8baf-aef7838dd4ed' AND q BETWEEN 31 AND 34 AND r=16; UPDATE units SET q=30,r=16,status='positioned' WHERE id='216b8a37-4a90-48ec-a0ec-0acec7ae707b';

UPDATE settlement_goods SET amount=10000,rate=0,cap=1000000 WHERE settlement_id='936c0005-a121-43f7-b4dc-40fde6897abe'; DELETE FROM sea_storms WHERE world_id='530726e5-9a08-4279-8baf-aef7838dd4ed';

INSERT INTO units(world_id,owner_id,type,category,size,crew,hull,status,settlement_id,support_settlement_id,home_settlement_id,provisions,name) VALUES('530726e5-9a08-4279-8baf-aef7838dd4ed','23ca1cb1-4b82-47e6-8076-8c2f0e4dec0b','galley','naval',1,20,3,'garrison','936c0005-a121-43f7-b4dc-40fde6897abe','936c0005-a121-43f7-b4dc-40fde6897abe','936c0005-a121-43f7-b4dc-40fde6897abe',0,'Sea Palace') RETURNING id;

UPDATE worlds SET current_tick=103,last_tick_at=now()-interval '22 seconds' WHERE id='530726e5-9a08-4279-8baf-aef7838dd4ed'; UPDATE settlement_goods SET calc_tick=103 WHERE settlement_id='936c0005-a121-43f7-b4dc-40fde6897abe';
