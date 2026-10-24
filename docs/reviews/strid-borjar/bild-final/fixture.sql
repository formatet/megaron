UPDATE players SET wanax_name='Leandros' WHERE id='6e10b412-ee22-4398-8adb-322437a2a5e6'

UPDATE players SET wanax_name='Idomeneus' WHERE id='acbb04a6-0a44-4ca1-9012-210f8068702a'

UPDATE units SET q=0,r=59,status='positioned' WHERE id='151ad043-77ba-49ef-9b78-b0f93cd8ab6f'; UPDATE units SET q=1,r=59,status='positioned' WHERE id='50df0521-c336-486a-91b1-88755a885fcc'; UPDATE worlds SET current_tick=100,last_tick_at=now() WHERE id='4b7216f4-1f93-420a-b10f-973e96b250cf';

INSERT INTO units(world_id,owner_id,type,category,size,status,q,r) VALUES('4b7216f4-1f93-420a-b10f-973e96b250cf','6e10b412-ee22-4398-8adb-322437a2a5e6','spearman','land',100,'positioned',0,59) RETURNING id

INSERT INTO units(world_id,owner_id,type,category,size,status,q,r,target_q,target_r,capture_mode) VALUES('4b7216f4-1f93-420a-b10f-973e96b250cf','acbb04a6-0a44-4ca1-9012-210f8068702a','spearman','land',100,'marching',1,59,0,59,'sack') RETURNING id

INSERT INTO scheduled_events(world_id,event_type,payload,process_after,due_tick) VALUES('4b7216f4-1f93-420a-b10f-973e96b250cf','UnitArrival','{"unit_id":"3c4fe6e6-92a0-44ba-b479-b5f588e62157","world_id":"4b7216f4-1f93-420a-b10f-973e96b250cf"}',now(),100);
