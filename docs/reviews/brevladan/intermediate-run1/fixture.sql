UPDATE players SET wanax_name='Leandros' WHERE id='052560a0-5f11-43f2-91e4-140af8652d3e';

UPDATE players SET wanax_name='Myrine' WHERE id='0f3227cd-214b-4643-8762-a616a48ed574';

UPDATE players SET wanax_name='Dorian' WHERE id='d5f5c6dd-119c-4ddd-a349-1e2c56c65ccc';

UPDATE players SET wanax_name='Thestor' WHERE id='28931a69-b07a-4c0a-b32d-13a68f22510d';

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('3947c685-a157-4f2b-9c62-7052caf1d343','052560a0-5f11-43f2-91e4-140af8652d3e','dc724343-019d-49f6-ad9e-10427166458d') ON CONFLICT DO NOTHING;

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('3947c685-a157-4f2b-9c62-7052caf1d343','052560a0-5f11-43f2-91e4-140af8652d3e','705ab552-c416-4aa1-b6ca-ed65c09d1e96') ON CONFLICT DO NOTHING;

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('3947c685-a157-4f2b-9c62-7052caf1d343','052560a0-5f11-43f2-91e4-140af8652d3e','fb5d88e5-8da1-449b-9b05-6e8e9413f0be') ON CONFLICT DO NOTHING;

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('3947c685-a157-4f2b-9c62-7052caf1d343','0f3227cd-214b-4643-8762-a616a48ed574','35b4f6bf-a49b-423b-9d1a-09379b6853eb') ON CONFLICT DO NOTHING;

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('3947c685-a157-4f2b-9c62-7052caf1d343','0f3227cd-214b-4643-8762-a616a48ed574','705ab552-c416-4aa1-b6ca-ed65c09d1e96') ON CONFLICT DO NOTHING;

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('3947c685-a157-4f2b-9c62-7052caf1d343','0f3227cd-214b-4643-8762-a616a48ed574','fb5d88e5-8da1-449b-9b05-6e8e9413f0be') ON CONFLICT DO NOTHING;

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('3947c685-a157-4f2b-9c62-7052caf1d343','d5f5c6dd-119c-4ddd-a349-1e2c56c65ccc','35b4f6bf-a49b-423b-9d1a-09379b6853eb') ON CONFLICT DO NOTHING;

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('3947c685-a157-4f2b-9c62-7052caf1d343','d5f5c6dd-119c-4ddd-a349-1e2c56c65ccc','dc724343-019d-49f6-ad9e-10427166458d') ON CONFLICT DO NOTHING;

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('3947c685-a157-4f2b-9c62-7052caf1d343','d5f5c6dd-119c-4ddd-a349-1e2c56c65ccc','fb5d88e5-8da1-449b-9b05-6e8e9413f0be') ON CONFLICT DO NOTHING;

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('3947c685-a157-4f2b-9c62-7052caf1d343','28931a69-b07a-4c0a-b32d-13a68f22510d','35b4f6bf-a49b-423b-9d1a-09379b6853eb') ON CONFLICT DO NOTHING;

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('3947c685-a157-4f2b-9c62-7052caf1d343','28931a69-b07a-4c0a-b32d-13a68f22510d','dc724343-019d-49f6-ad9e-10427166458d') ON CONFLICT DO NOTHING;

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('3947c685-a157-4f2b-9c62-7052caf1d343','28931a69-b07a-4c0a-b32d-13a68f22510d','705ab552-c416-4aa1-b6ca-ed65c09d1e96') ON CONFLICT DO NOTHING;

UPDATE worlds SET current_tick=100,last_tick_at=now() WHERE id='3947c685-a157-4f2b-9c62-7052caf1d343'; UPDATE settlement_goods SET amount=10000,rate=0,calc_tick=100 WHERE settlement_id IN('8f3fb5e4-e327-4398-8078-9f269a0d0aea','d714836d-3bc2-4bac-8094-8e93ef9507eb','ca01a2b0-00ab-4fe6-a563-b224d52e6fdc','14610069-b027-42a4-8507-732b710c4959') AND good_key IN('grain','silver','copper');

UPDATE scheduled_events SET due_tick=100,process_after=now() WHERE world_id='3947c685-a157-4f2b-9c62-7052caf1d343' AND event_type='MessengerArrival' AND payload->>'messenger_id' IN('cfdbcecd-71b7-47f1-9f3c-b3120c077b84','6dd037e0-eee3-4596-b573-2864c8cc8bf6','6ccc717b-caa1-4078-bf04-091ec5d15c92');
