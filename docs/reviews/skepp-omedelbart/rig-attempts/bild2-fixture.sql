UPDATE map_tiles SET terrain='plains' WHERE world_id='9f9d75d1-b8bc-4bcb-bb79-8472a93d5b6d'; UPDATE map_tiles SET terrain='coastal_sea' WHERE world_id='9f9d75d1-b8bc-4bcb-bb79-8472a93d5b6d' AND q BETWEEN 23 AND 26 AND r=16; UPDATE units SET q=22,r=16,status='positioned' WHERE id='62a7d633-7659-4aad-8e27-61568e52599c';

UPDATE settlement_goods SET amount=10000,rate=0,cap=1000000 WHERE settlement_id='35c0b70e-6963-47a9-b41c-d1acd63c4b79'; DELETE FROM sea_storms WHERE world_id='9f9d75d1-b8bc-4bcb-bb79-8472a93d5b6d';

INSERT INTO units(world_id,owner_id,type,category,size,crew,hull,status,settlement_id,support_settlement_id,home_settlement_id,provisions,name) VALUES('9f9d75d1-b8bc-4bcb-bb79-8472a93d5b6d','292b2cee-8e9e-47de-b837-06852a66f737','galley','naval',1,20,5,'garrison','35c0b70e-6963-47a9-b41c-d1acd63c4b79','35c0b70e-6963-47a9-b41c-d1acd63c4b79','35c0b70e-6963-47a9-b41c-d1acd63c4b79',100,'Sea Palace') RETURNING id;

UPDATE worlds SET current_tick=103,last_tick_at=now()-interval '22 seconds' WHERE id='9f9d75d1-b8bc-4bcb-bb79-8472a93d5b6d'; UPDATE settlement_goods SET calc_tick=103 WHERE settlement_id='35c0b70e-6963-47a9-b41c-d1acd63c4b79';
