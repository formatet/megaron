UPDATE players SET wanax_name='Leandros' WHERE id='8de33e02-f980-426c-9390-7f04179b6d55';

UPDATE players SET wanax_name='Myrine' WHERE id='aed24026-9597-4977-854b-ff83a29a295f';

UPDATE players SET wanax_name='Dorian' WHERE id='cd792ca3-25df-44a7-bda5-c83bb39aedca';

UPDATE players SET wanax_name='Thestor' WHERE id='80ccf863-673b-41e7-97e7-1fed579ffb97';

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('88d5c02f-6632-4c2b-b0ce-4f3e868fda6e','8de33e02-f980-426c-9390-7f04179b6d55','0d1004d9-5c95-431a-b51c-d957fa2cad88') ON CONFLICT DO NOTHING;

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('88d5c02f-6632-4c2b-b0ce-4f3e868fda6e','8de33e02-f980-426c-9390-7f04179b6d55','19f63872-bf7c-452d-bb9a-776a5c71b448') ON CONFLICT DO NOTHING;

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('88d5c02f-6632-4c2b-b0ce-4f3e868fda6e','8de33e02-f980-426c-9390-7f04179b6d55','be6d8fa6-8afc-4f8a-8ef0-5eb81750c4d2') ON CONFLICT DO NOTHING;

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('88d5c02f-6632-4c2b-b0ce-4f3e868fda6e','aed24026-9597-4977-854b-ff83a29a295f','211a0f19-b206-42bd-8b0d-c370f38ffa4c') ON CONFLICT DO NOTHING;

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('88d5c02f-6632-4c2b-b0ce-4f3e868fda6e','aed24026-9597-4977-854b-ff83a29a295f','19f63872-bf7c-452d-bb9a-776a5c71b448') ON CONFLICT DO NOTHING;

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('88d5c02f-6632-4c2b-b0ce-4f3e868fda6e','aed24026-9597-4977-854b-ff83a29a295f','be6d8fa6-8afc-4f8a-8ef0-5eb81750c4d2') ON CONFLICT DO NOTHING;

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('88d5c02f-6632-4c2b-b0ce-4f3e868fda6e','cd792ca3-25df-44a7-bda5-c83bb39aedca','211a0f19-b206-42bd-8b0d-c370f38ffa4c') ON CONFLICT DO NOTHING;

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('88d5c02f-6632-4c2b-b0ce-4f3e868fda6e','cd792ca3-25df-44a7-bda5-c83bb39aedca','0d1004d9-5c95-431a-b51c-d957fa2cad88') ON CONFLICT DO NOTHING;

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('88d5c02f-6632-4c2b-b0ce-4f3e868fda6e','cd792ca3-25df-44a7-bda5-c83bb39aedca','be6d8fa6-8afc-4f8a-8ef0-5eb81750c4d2') ON CONFLICT DO NOTHING;

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('88d5c02f-6632-4c2b-b0ce-4f3e868fda6e','80ccf863-673b-41e7-97e7-1fed579ffb97','211a0f19-b206-42bd-8b0d-c370f38ffa4c') ON CONFLICT DO NOTHING;

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('88d5c02f-6632-4c2b-b0ce-4f3e868fda6e','80ccf863-673b-41e7-97e7-1fed579ffb97','0d1004d9-5c95-431a-b51c-d957fa2cad88') ON CONFLICT DO NOTHING;

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('88d5c02f-6632-4c2b-b0ce-4f3e868fda6e','80ccf863-673b-41e7-97e7-1fed579ffb97','19f63872-bf7c-452d-bb9a-776a5c71b448') ON CONFLICT DO NOTHING;

UPDATE worlds SET current_tick=100,last_tick_at=now() WHERE id='88d5c02f-6632-4c2b-b0ce-4f3e868fda6e'; UPDATE settlement_goods SET amount=10000,rate=0,calc_tick=100 WHERE settlement_id IN('4885430c-53cd-4466-9e5c-9adbc63a817d','523a932a-1098-4c47-9031-e9108525a998','41d1a869-4b2d-4d92-b866-a39a37963376','5ea8a0b1-cc0b-4746-95a4-6336fb3e7697') AND good_key IN('grain','silver','copper');

UPDATE scheduled_events SET due_tick=100,process_after=now() WHERE world_id='88d5c02f-6632-4c2b-b0ce-4f3e868fda6e' AND event_type='MessengerArrival' AND payload->>'messenger_id' IN('f5d14571-10d7-410a-8c04-8b1efcaf4e55','96167b1c-c42c-465d-b8ad-8da75d3d1262','6eefc97b-7b74-4133-8da6-4cf50c251aaa');
