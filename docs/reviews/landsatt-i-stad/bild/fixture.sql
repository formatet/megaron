UPDATE map_tiles SET terrain='plains',coastal=false WHERE world_id='30dc6ee8-6ff5-43de-a599-a12e49155b3f'; UPDATE units SET q=9,r=23,status='positioned' WHERE id='bd38f65b-a966-494b-adf7-91b3bcead664';

INSERT INTO provinces(world_id,map_q,map_r,terrain_type,coastal) VALUES('30dc6ee8-6ff5-43de-a599-a12e49155b3f',15,23,'plains',true) RETURNING id;

INSERT INTO settlements(world_id,province_id,name,culture_id,owner_id,control_type,is_capital,state,population) VALUES('30dc6ee8-6ff5-43de-a599-a12e49155b3f','e296739b-421c-49c4-bd36-f73aae5cff0e','Petras','achaean','29fcf05b-aedf-4716-ba0f-4e6423aece5b','colony',false,'active',5000) RETURNING id;

UPDATE provinces SET coastal=true WHERE id IN ('5c4d9be9-559f-468c-8454-d7eb5935cc7b','e296739b-421c-49c4-bd36-f73aae5cff0e'); UPDATE map_tiles SET terrain='coastal_sea' WHERE world_id='30dc6ee8-6ff5-43de-a599-a12e49155b3f' AND r=23 AND q BETWEEN 10 AND 14; UPDATE map_tiles SET coastal=true WHERE world_id='30dc6ee8-6ff5-43de-a599-a12e49155b3f' AND r=23 AND q IN (9,15); DELETE FROM sea_storms WHERE world_id='30dc6ee8-6ff5-43de-a599-a12e49155b3f'; UPDATE settlements SET population=5000 WHERE id='139ea21a-26fa-461b-89ae-0bd76747b0fe'; UPDATE settlement_goods SET amount=100000,rate=0,cap=1000000,calc_tick=current_world_tick() WHERE settlement_id='139ea21a-26fa-461b-89ae-0bd76747b0fe'; INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) SELECT '38e12ae7-7a58-41ae-a4f9-8a867077ab0b',key,100000,0,1000000,current_world_tick() FROM goods ON CONFLICT DO NOTHING;

INSERT INTO units(world_id,owner_id,type,category,size,crew,hull,status,q,r,settlement_id,support_settlement_id,name) VALUES('30dc6ee8-6ff5-43de-a599-a12e49155b3f','29fcf05b-aedf-4716-ba0f-4e6423aece5b','merchantman','naval',1,10,5,'garrison',9,23,'139ea21a-26fa-461b-89ae-0bd76747b0fe','139ea21a-26fa-461b-89ae-0bd76747b0fe','Petras Carrier') RETURNING id;

INSERT INTO units(world_id,owner_id,type,category,size,status,q,r,settlement_id,support_settlement_id,ordinal) VALUES('30dc6ee8-6ff5-43de-a599-a12e49155b3f','29fcf05b-aedf-4716-ba0f-4e6423aece5b','spearman','land',100,'garrison',9,23,'139ea21a-26fa-461b-89ae-0bd76747b0fe','139ea21a-26fa-461b-89ae-0bd76747b0fe',3) RETURNING id;

INSERT INTO units(world_id,owner_id,type,category,size,status,q,r,settlement_id,support_settlement_id,ordinal) VALUES('30dc6ee8-6ff5-43de-a599-a12e49155b3f','29fcf05b-aedf-4716-ba0f-4e6423aece5b','spearman','land',100,'garrison',9,23,'139ea21a-26fa-461b-89ae-0bd76747b0fe','139ea21a-26fa-461b-89ae-0bd76747b0fe',4) RETURNING id;

INSERT INTO units(world_id,owner_id,type,category,size,status,q,r,settlement_id,support_settlement_id,ordinal) VALUES('30dc6ee8-6ff5-43de-a599-a12e49155b3f','29fcf05b-aedf-4716-ba0f-4e6423aece5b','spearman','land',100,'garrison',9,23,'139ea21a-26fa-461b-89ae-0bd76747b0fe','139ea21a-26fa-461b-89ae-0bd76747b0fe',5) RETURNING id;
