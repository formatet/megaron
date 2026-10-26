UPDATE map_tiles SET terrain='plains',coastal=false WHERE world_id='e9070aa9-f356-4fdc-b738-6a09b9598457'; UPDATE provinces SET terrain_type='plains',coastal=false WHERE world_id='e9070aa9-f356-4fdc-b738-6a09b9598457'; UPDATE units SET q=6,r=24,status='positioned' WHERE id='59a17ed7-cb1b-4e58-924e-c578939d1836';

UPDATE settlements SET population=4000 WHERE id='6af22e2d-7cea-4d2b-9f66-989304b2d4b6'; UPDATE settlement_goods SET amount=10000,rate=0,cap=1000000,calc_tick=current_world_tick() WHERE settlement_id='6af22e2d-7cea-4d2b-9f66-989304b2d4b6'; UPDATE units SET status='disbanded' WHERE world_id='e9070aa9-f356-4fdc-b738-6a09b9598457' AND owner_id='ff971c79-64ec-40bd-bbbb-09473a649617';

UPDATE units SET status='disbanded' WHERE world_id='e9070aa9-f356-4fdc-b738-6a09b9598457' AND owner_id='ff971c79-64ec-40bd-bbbb-09473a649617';

INSERT INTO units(world_id,owner_id,settlement_id,support_settlement_id,type,category,size,status,q,r,ordinal) VALUES('e9070aa9-f356-4fdc-b738-6a09b9598457','ff971c79-64ec-40bd-bbbb-09473a649617',NULL,'6af22e2d-7cea-4d2b-9f66-989304b2d4b6','spearman','land',100,'positioned',8,24,1) RETURNING id;

INSERT INTO units(world_id,owner_id,settlement_id,support_settlement_id,type,category,size,status,q,r,ordinal) VALUES('e9070aa9-f356-4fdc-b738-6a09b9598457','ff971c79-64ec-40bd-bbbb-09473a649617',NULL,'6af22e2d-7cea-4d2b-9f66-989304b2d4b6','spearman','land',100,'positioned',8,24,2) RETURNING id;

INSERT INTO units(world_id,owner_id,settlement_id,support_settlement_id,type,category,size,status,q,r,ordinal) VALUES('e9070aa9-f356-4fdc-b738-6a09b9598457','ff971c79-64ec-40bd-bbbb-09473a649617',NULL,'6af22e2d-7cea-4d2b-9f66-989304b2d4b6','spearman','land',100,'positioned',8,24,3) RETURNING id;

UPDATE units SET status='disbanded' WHERE world_id='e9070aa9-f356-4fdc-b738-6a09b9598457' AND owner_id='ff971c79-64ec-40bd-bbbb-09473a649617';

INSERT INTO units(world_id,owner_id,settlement_id,support_settlement_id,type,category,size,status,q,r,ordinal) VALUES('e9070aa9-f356-4fdc-b738-6a09b9598457','ff971c79-64ec-40bd-bbbb-09473a649617',NULL,'6af22e2d-7cea-4d2b-9f66-989304b2d4b6','spearman','land',100,'positioned',8,24,1) RETURNING id;

INSERT INTO units(world_id,owner_id,settlement_id,support_settlement_id,type,category,size,status,q,r,ordinal) VALUES('e9070aa9-f356-4fdc-b738-6a09b9598457','ff971c79-64ec-40bd-bbbb-09473a649617',NULL,'6af22e2d-7cea-4d2b-9f66-989304b2d4b6','spearman','land',100,'positioned',8,24,2) RETURNING id;

INSERT INTO units(world_id,owner_id,settlement_id,support_settlement_id,type,category,size,status,q,r,ordinal) VALUES('e9070aa9-f356-4fdc-b738-6a09b9598457','ff971c79-64ec-40bd-bbbb-09473a649617',NULL,'6af22e2d-7cea-4d2b-9f66-989304b2d4b6','spearman','land',100,'positioned',8,24,3) RETURNING id;
