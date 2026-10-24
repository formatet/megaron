UPDATE players SET wanax_name='Leandros' WHERE id='3f2a0f42-6f7e-4c02-a3c6-21f674809a27'

UPDATE players SET wanax_name='Myrine' WHERE id='a530f220-59df-4195-9cd0-db9fa0e8258c'

UPDATE map_tiles SET terrain='plains',coastal=true WHERE world_id='a6e0adb6-8c54-4333-9846-40c6ac32fe99' AND r=23 AND q BETWEEN 9 AND 15; UPDATE map_tiles SET terrain='coastal_sea' WHERE world_id='a6e0adb6-8c54-4333-9846-40c6ac32fe99' AND r=24 AND q BETWEEN 8 AND 16;

UPDATE units SET q=9,r=23,status='positioned' WHERE id='5c21d384-c265-4df0-947a-ed4590a5f3ea';

UPDATE units SET q=15,r=23,status='positioned' WHERE id='afa91b33-5e72-4738-abbd-04863f149b02';

UPDATE worlds SET current_tick=100,last_tick_at=now() WHERE id='a6e0adb6-8c54-4333-9846-40c6ac32fe99'; UPDATE provinces SET coastal=true WHERE id IN('00572915-836c-422d-906d-7d6ce294f81f','ab497f73-a37a-4982-ab5b-7961e571cb56'); UPDATE settlements SET owner_id='3f2a0f42-6f7e-4c02-a3c6-21f674809a27' WHERE id='23e64245-c47f-4576-b447-d28efc683ee7'; UPDATE units SET status='disbanded' WHERE world_id='a6e0adb6-8c54-4333-9846-40c6ac32fe99' AND owner_id='a530f220-59df-4195-9cd0-db9fa0e8258c'; UPDATE units SET q=9,r=23,status='garrison',settlement_id='46561e5d-e45d-4094-a650-8e0b3c707f5b',support_settlement_id='46561e5d-e45d-4094-a650-8e0b3c707f5b' WHERE world_id='a6e0adb6-8c54-4333-9846-40c6ac32fe99' AND owner_id='3f2a0f42-6f7e-4c02-a3c6-21f674809a27' AND type='spearman'; UPDATE settlement_goods SET amount=10000,rate=0,cap=1000000,calc_tick=100 WHERE settlement_id IN('46561e5d-e45d-4094-a650-8e0b3c707f5b','23e64245-c47f-4576-b447-d28efc683ee7'); INSERT INTO buildings(settlement_id,building_type,level) VALUES('46561e5d-e45d-4094-a650-8e0b3c707f5b','harbour',1),('23e64245-c47f-4576-b447-d28efc683ee7','harbour',1) ON CONFLICT DO NOTHING; DELETE FROM sea_storms WHERE world_id='a6e0adb6-8c54-4333-9846-40c6ac32fe99';

UPDATE units SET status='disbanded' WHERE world_id='a6e0adb6-8c54-4333-9846-40c6ac32fe99' AND category='naval';

INSERT INTO units(world_id,owner_id,type,category,size,crew,status,settlement_id,support_settlement_id,q,r,name,hull) VALUES('a6e0adb6-8c54-4333-9846-40c6ac32fe99','3f2a0f42-6f7e-4c02-a3c6-21f674809a27','galley','naval',1,20,'garrison','46561e5d-e45d-4094-a650-8e0b3c707f5b','46561e5d-e45d-4094-a650-8e0b3c707f5b',9,23,'White Dolphin',5) RETURNING id

INSERT INTO units(world_id,owner_id,type,category,size,crew,status,settlement_id,support_settlement_id,q,r,name,hull) VALUES('a6e0adb6-8c54-4333-9846-40c6ac32fe99','3f2a0f42-6f7e-4c02-a3c6-21f674809a27','galley','naval',1,20,'garrison','46561e5d-e45d-4094-a650-8e0b3c707f5b','46561e5d-e45d-4094-a650-8e0b3c707f5b',9,23,'Blue Heron',5) RETURNING id

INSERT INTO units(world_id,owner_id,type,category,size,crew,status,settlement_id,support_settlement_id,q,r,name,hull) VALUES('a6e0adb6-8c54-4333-9846-40c6ac32fe99','3f2a0f42-6f7e-4c02-a3c6-21f674809a27','merchantman','naval',1,10,'garrison','46561e5d-e45d-4094-a650-8e0b3c707f5b','46561e5d-e45d-4094-a650-8e0b3c707f5b',9,23,'Amber Bird',5)
