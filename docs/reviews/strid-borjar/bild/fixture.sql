UPDATE players SET wanax_name='Leandros' WHERE id='bc389317-d3c4-453f-b93c-35f4d867e739'

UPDATE players SET wanax_name='Idomeneus' WHERE id='cbb72cb2-f7ac-4f6e-b7ed-f7b546c2ce9d'

UPDATE units SET q=0,r=53,status='positioned' WHERE id='8a7a5ed6-fa77-4961-ab04-198bc4c9ce09'; UPDATE units SET q=1,r=53,status='positioned' WHERE id='6a1cdf60-71fa-46cf-a2e9-f260daee012f'; UPDATE worlds SET current_tick=100,last_tick_at=now() WHERE id='1dd2a620-9977-4766-9281-ec62eada5c6b';

INSERT INTO units(world_id,owner_id,type,category,size,status,q,r) VALUES('1dd2a620-9977-4766-9281-ec62eada5c6b','bc389317-d3c4-453f-b93c-35f4d867e739','spearman','land',100,'positioned',0,53) RETURNING id

INSERT INTO units(world_id,owner_id,type,category,size,status,q,r,target_q,target_r,capture_mode) VALUES('1dd2a620-9977-4766-9281-ec62eada5c6b','cbb72cb2-f7ac-4f6e-b7ed-f7b546c2ce9d','spearman','land',100,'marching',1,53,0,53,'sack') RETURNING id

INSERT INTO scheduled_events(world_id,event_type,payload,process_after,due_tick) VALUES('1dd2a620-9977-4766-9281-ec62eada5c6b','UnitArrival','{"unit_id":"c757150a-c98b-4912-b2a4-7c6f60fcbf50","world_id":"1dd2a620-9977-4766-9281-ec62eada5c6b"}',now(),100);
