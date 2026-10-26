UPDATE map_tiles SET terrain='plains',coastal=false WHERE world_id='93dfab27-5652-4be7-98b0-a07917b0114c'; UPDATE provinces SET terrain_type='plains',coastal=false WHERE world_id='93dfab27-5652-4be7-98b0-a07917b0114c'; UPDATE units SET q=6,r=24,status='positioned' WHERE id='44490033-a8e1-4d9c-935a-bd5c1c754a04';

UPDATE settlements SET population=4000 WHERE id='991af584-124e-40a9-a51a-7b406f53f82d'; UPDATE settlement_goods SET amount=10000,rate=0,cap=1000000,calc_tick=current_world_tick() WHERE settlement_id='991af584-124e-40a9-a51a-7b406f53f82d'; UPDATE units SET status='disbanded' WHERE world_id='93dfab27-5652-4be7-98b0-a07917b0114c' AND owner_id='b3767620-cc20-428d-9a5f-1c6c3f93cab4';

UPDATE units SET status='disbanded' WHERE world_id='93dfab27-5652-4be7-98b0-a07917b0114c' AND owner_id='b3767620-cc20-428d-9a5f-1c6c3f93cab4';

INSERT INTO units(world_id,owner_id,settlement_id,support_settlement_id,type,category,size,status,q,r,ordinal) VALUES('93dfab27-5652-4be7-98b0-a07917b0114c','b3767620-cc20-428d-9a5f-1c6c3f93cab4',NULL,'991af584-124e-40a9-a51a-7b406f53f82d','spearman','land',100,'positioned',8,24,1) RETURNING id;

INSERT INTO units(world_id,owner_id,settlement_id,support_settlement_id,type,category,size,status,q,r,ordinal) VALUES('93dfab27-5652-4be7-98b0-a07917b0114c','b3767620-cc20-428d-9a5f-1c6c3f93cab4',NULL,'991af584-124e-40a9-a51a-7b406f53f82d','spearman','land',100,'positioned',8,24,2) RETURNING id;

INSERT INTO units(world_id,owner_id,settlement_id,support_settlement_id,type,category,size,status,q,r,ordinal) VALUES('93dfab27-5652-4be7-98b0-a07917b0114c','b3767620-cc20-428d-9a5f-1c6c3f93cab4',NULL,'991af584-124e-40a9-a51a-7b406f53f82d','spearman','land',100,'positioned',8,24,3) RETURNING id;

UPDATE units SET status='disbanded' WHERE world_id='93dfab27-5652-4be7-98b0-a07917b0114c' AND owner_id='b3767620-cc20-428d-9a5f-1c6c3f93cab4';

INSERT INTO units(world_id,owner_id,settlement_id,support_settlement_id,type,category,size,status,q,r,ordinal) VALUES('93dfab27-5652-4be7-98b0-a07917b0114c','b3767620-cc20-428d-9a5f-1c6c3f93cab4',NULL,'991af584-124e-40a9-a51a-7b406f53f82d','spearman','land',100,'positioned',8,24,1) RETURNING id;

INSERT INTO units(world_id,owner_id,settlement_id,support_settlement_id,type,category,size,status,q,r,ordinal) VALUES('93dfab27-5652-4be7-98b0-a07917b0114c','b3767620-cc20-428d-9a5f-1c6c3f93cab4',NULL,'991af584-124e-40a9-a51a-7b406f53f82d','spearman','land',100,'positioned',8,24,2) RETURNING id;

INSERT INTO units(world_id,owner_id,settlement_id,support_settlement_id,type,category,size,status,q,r,ordinal) VALUES('93dfab27-5652-4be7-98b0-a07917b0114c','b3767620-cc20-428d-9a5f-1c6c3f93cab4',NULL,'991af584-124e-40a9-a51a-7b406f53f82d','spearman','land',100,'positioned',8,24,3) RETURNING id;
