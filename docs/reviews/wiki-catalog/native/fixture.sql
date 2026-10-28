UPDATE map_tiles SET terrain='coastal_sea',coastal=false WHERE world_id='a3f6c5d8-72ff-4d70-89fa-d58eea4ea8ba'; UPDATE map_tiles SET terrain='plains' WHERE world_id='a3f6c5d8-72ff-4d70-89fa-d58eea4ea8ba' AND q BETWEEN 5 AND 11 AND r BETWEEN 25 AND 28; UPDATE map_tiles SET terrain='plains',coastal=true WHERE world_id='a3f6c5d8-72ff-4d70-89fa-d58eea4ea8ba' AND q=18 AND r=25; UPDATE map_tiles SET coastal=true WHERE world_id='a3f6c5d8-72ff-4d70-89fa-d58eea4ea8ba' AND q=5 AND r=25; UPDATE units SET q=5,r=25,status='positioned' WHERE id='44065344-4790-4804-8b79-35b642e150f0'; DELETE FROM sea_storms WHERE world_id='a3f6c5d8-72ff-4d70-89fa-d58eea4ea8ba';

INSERT INTO units(world_id,owner_id,type,category,size,ordinal,status,q,r) VALUES('a3f6c5d8-72ff-4d70-89fa-d58eea4ea8ba','f7171596-f00b-4a99-adfe-158409627ba5','elite_infantry','land',100,1,'positioned',5,25);

INSERT INTO units(world_id,owner_id,type,category,size,ordinal,status,q,r) VALUES('a3f6c5d8-72ff-4d70-89fa-d58eea4ea8ba','f7171596-f00b-4a99-adfe-158409627ba5','war_chariot','land',100,1,'positioned',5,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('a3f6c5d8-72ff-4d70-89fa-d58eea4ea8ba','f7171596-f00b-4a99-adfe-158409627ba5','merchantman','naval',1,10,'Cargo 0','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('a3f6c5d8-72ff-4d70-89fa-d58eea4ea8ba','f7171596-f00b-4a99-adfe-158409627ba5','merchantman','naval',1,10,'Cargo 1','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('a3f6c5d8-72ff-4d70-89fa-d58eea4ea8ba','f7171596-f00b-4a99-adfe-158409627ba5','merchantman','naval',1,10,'Cargo 2','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('a3f6c5d8-72ff-4d70-89fa-d58eea4ea8ba','f7171596-f00b-4a99-adfe-158409627ba5','merchantman','naval',1,10,'Cargo 3','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('a3f6c5d8-72ff-4d70-89fa-d58eea4ea8ba','f7171596-f00b-4a99-adfe-158409627ba5','merchantman','naval',1,10,'Cargo 4','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('a3f6c5d8-72ff-4d70-89fa-d58eea4ea8ba','f7171596-f00b-4a99-adfe-158409627ba5','merchantman','naval',1,10,'Cargo 5','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('a3f6c5d8-72ff-4d70-89fa-d58eea4ea8ba','f7171596-f00b-4a99-adfe-158409627ba5','merchantman','naval',1,10,'Cargo 6','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('a3f6c5d8-72ff-4d70-89fa-d58eea4ea8ba','f7171596-f00b-4a99-adfe-158409627ba5','merchantman','naval',1,10,'Cargo 7','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('a3f6c5d8-72ff-4d70-89fa-d58eea4ea8ba','f7171596-f00b-4a99-adfe-158409627ba5','merchantman','naval',1,10,'Cargo 8','positioned',4,25);

INSERT INTO units(world_id,owner_id,type,category,size,crew,name,status,q,r) VALUES('a3f6c5d8-72ff-4d70-89fa-d58eea4ea8ba','f7171596-f00b-4a99-adfe-158409627ba5','merchantman','naval',1,10,'Cargo 9','positioned',4,25);

UPDATE provinces SET coastal=true WHERE id='7f005093-2c32-465c-b2da-6f2b42473d45'; UPDATE settlements SET population=4000 WHERE id='0058b17c-57f5-4899-861e-4704d90db7ce'; INSERT INTO buildings(settlement_id,building_type,level) VALUES('0058b17c-57f5-4899-861e-4704d90db7ce','harbour',1) ON CONFLICT DO NOTHING; UPDATE units SET settlement_id='0058b17c-57f5-4899-861e-4704d90db7ce',support_settlement_id='0058b17c-57f5-4899-861e-4704d90db7ce',status='garrison',q=NULL,r=NULL WHERE world_id='a3f6c5d8-72ff-4d70-89fa-d58eea4ea8ba' AND owner_id='f7171596-f00b-4a99-adfe-158409627ba5' AND type<>'nomadic_host' AND status<>'disbanded';
