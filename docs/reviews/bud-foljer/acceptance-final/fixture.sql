UPDATE map_tiles SET terrain='plains',coastal=false WHERE world_id='28535f9c-c8e3-4cd7-afca-c698641875dd'; UPDATE provinces SET terrain_type='plains',coastal=false WHERE world_id='28535f9c-c8e3-4cd7-afca-c698641875dd'; UPDATE units SET q=6,r=24,status='positioned' WHERE id='41302b43-226e-4930-9598-476d70e43196';

UPDATE settlements SET population=4000 WHERE id='7b338c2c-9a7b-4313-b34b-90f84477e33c'; UPDATE settlement_goods SET amount=10000,rate=0,cap=1000000,calc_tick=current_world_tick() WHERE settlement_id='7b338c2c-9a7b-4313-b34b-90f84477e33c'; UPDATE units SET status='disbanded' WHERE world_id='28535f9c-c8e3-4cd7-afca-c698641875dd' AND owner_id='69b68583-7a82-413a-9478-1c67a1245c58';

INSERT INTO player_scouted_tiles(world_id,player_id,q,r) SELECT world_id,'69b68583-7a82-413a-9478-1c67a1245c58',q,r FROM map_tiles WHERE world_id='28535f9c-c8e3-4cd7-afca-c698641875dd' ON CONFLICT DO NOTHING;
