UPDATE map_tiles SET terrain='plains',coastal=false WHERE world_id='f185f1bf-bf91-4bd7-8024-05c3eed32e4a'; UPDATE provinces SET terrain_type='plains',coastal=false WHERE world_id='f185f1bf-bf91-4bd7-8024-05c3eed32e4a'; UPDATE units SET q=6,r=24,status='positioned' WHERE id='d0decd13-de29-49c6-8fbf-65454e254f70';

UPDATE settlements SET population=4000 WHERE id='6af6660f-b65e-435c-a8a3-c7d87f8276c2'; UPDATE settlement_goods SET amount=10000,rate=0,cap=1000000,calc_tick=current_world_tick() WHERE settlement_id='6af6660f-b65e-435c-a8a3-c7d87f8276c2'; UPDATE units SET status='disbanded' WHERE world_id='f185f1bf-bf91-4bd7-8024-05c3eed32e4a' AND owner_id='9e4e6806-9a47-4cc8-860b-64ca3e21f1aa';

INSERT INTO player_scouted_tiles(world_id,player_id,q,r) SELECT world_id,'9e4e6806-9a47-4cc8-860b-64ca3e21f1aa',q,r FROM map_tiles WHERE world_id='f185f1bf-bf91-4bd7-8024-05c3eed32e4a' ON CONFLICT DO NOTHING;
