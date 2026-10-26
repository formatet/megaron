UPDATE map_tiles SET terrain='plains',coastal=false WHERE world_id='0a48275d-a7d9-42ea-a11d-efa7d2536fc4'; UPDATE provinces SET terrain_type='plains',coastal=false WHERE world_id='0a48275d-a7d9-42ea-a11d-efa7d2536fc4'; UPDATE units SET q=6,r=24,status='positioned' WHERE id='8c6cb633-e7a7-45fd-ba7d-50110a5cf182';

UPDATE settlements SET population=4000 WHERE id='08d219d0-465d-4846-a00a-2af9248bc9c2'; UPDATE settlement_goods SET amount=10000,rate=0,cap=1000000,calc_tick=current_world_tick() WHERE settlement_id='08d219d0-465d-4846-a00a-2af9248bc9c2'; UPDATE units SET status='disbanded' WHERE world_id='0a48275d-a7d9-42ea-a11d-efa7d2536fc4' AND owner_id='34327926-4bef-454b-a41b-0b043ea6da95';

INSERT INTO player_scouted_tiles(world_id,player_id,q,r) SELECT world_id,'34327926-4bef-454b-a41b-0b043ea6da95',q,r FROM map_tiles WHERE world_id='0a48275d-a7d9-42ea-a11d-efa7d2536fc4' ON CONFLICT DO NOTHING;
