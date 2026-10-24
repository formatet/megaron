UPDATE players SET wanax_name='Leandros' WHERE id='2c2c76f3-d465-4f82-83c4-4f6477a2ce04';

UPDATE players SET wanax_name='Myrine' WHERE id='befbcc44-e50f-44ec-a77e-e33dc85ee272';

UPDATE players SET wanax_name='Dorian' WHERE id='ec37cf50-8ddc-4eb8-953c-4d6235325ed7';

UPDATE players SET wanax_name='Thestor' WHERE id='94456dfe-4dfa-4e2d-8b18-41ce520182cc';

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('8f730778-0cf2-4602-964e-7f50c655f072','2c2c76f3-d465-4f82-83c4-4f6477a2ce04','ec4d8f62-dfdc-4572-b5da-8c3297debeb2') ON CONFLICT DO NOTHING;

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('8f730778-0cf2-4602-964e-7f50c655f072','2c2c76f3-d465-4f82-83c4-4f6477a2ce04','148e4b79-e9bd-44ed-ae7f-aa9274aa61e0') ON CONFLICT DO NOTHING;

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('8f730778-0cf2-4602-964e-7f50c655f072','2c2c76f3-d465-4f82-83c4-4f6477a2ce04','a7b0f178-e3aa-4091-be54-605cfbd7ab1b') ON CONFLICT DO NOTHING;

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('8f730778-0cf2-4602-964e-7f50c655f072','befbcc44-e50f-44ec-a77e-e33dc85ee272','c7039452-324c-4e52-bbbc-cf421a3a823a') ON CONFLICT DO NOTHING;

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('8f730778-0cf2-4602-964e-7f50c655f072','befbcc44-e50f-44ec-a77e-e33dc85ee272','148e4b79-e9bd-44ed-ae7f-aa9274aa61e0') ON CONFLICT DO NOTHING;

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('8f730778-0cf2-4602-964e-7f50c655f072','befbcc44-e50f-44ec-a77e-e33dc85ee272','a7b0f178-e3aa-4091-be54-605cfbd7ab1b') ON CONFLICT DO NOTHING;

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('8f730778-0cf2-4602-964e-7f50c655f072','ec37cf50-8ddc-4eb8-953c-4d6235325ed7','c7039452-324c-4e52-bbbc-cf421a3a823a') ON CONFLICT DO NOTHING;

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('8f730778-0cf2-4602-964e-7f50c655f072','ec37cf50-8ddc-4eb8-953c-4d6235325ed7','ec4d8f62-dfdc-4572-b5da-8c3297debeb2') ON CONFLICT DO NOTHING;

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('8f730778-0cf2-4602-964e-7f50c655f072','ec37cf50-8ddc-4eb8-953c-4d6235325ed7','a7b0f178-e3aa-4091-be54-605cfbd7ab1b') ON CONFLICT DO NOTHING;

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('8f730778-0cf2-4602-964e-7f50c655f072','94456dfe-4dfa-4e2d-8b18-41ce520182cc','c7039452-324c-4e52-bbbc-cf421a3a823a') ON CONFLICT DO NOTHING;

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('8f730778-0cf2-4602-964e-7f50c655f072','94456dfe-4dfa-4e2d-8b18-41ce520182cc','ec4d8f62-dfdc-4572-b5da-8c3297debeb2') ON CONFLICT DO NOTHING;

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('8f730778-0cf2-4602-964e-7f50c655f072','94456dfe-4dfa-4e2d-8b18-41ce520182cc','148e4b79-e9bd-44ed-ae7f-aa9274aa61e0') ON CONFLICT DO NOTHING;

UPDATE worlds SET current_tick=100,last_tick_at=now() WHERE id='8f730778-0cf2-4602-964e-7f50c655f072'; UPDATE settlement_goods SET amount=10000,rate=0,calc_tick=100 WHERE settlement_id IN('10390dd8-2f64-4177-899f-d4efe8908cce','45a17810-e655-45db-a6cf-8320581eef62','47ed2330-e5ca-47c4-be63-e1b26ec4fd64','0ec28f95-f293-4097-b38c-4f235f5f4d34') AND good_key IN('grain','silver','copper');

UPDATE scheduled_events SET due_tick=100,process_after=now() WHERE world_id='8f730778-0cf2-4602-964e-7f50c655f072' AND event_type='MessengerArrival' AND payload->>'messenger_id' IN('deba6f5c-e83d-4d7a-98de-37b0c5514188','bbd252d0-2c91-4d4f-abb6-5958e6a59150','7dbf5654-c49e-4638-86aa-8aff402af081');

INSERT INTO events(stream_id,stream_type,event_type,payload,world_id) VALUES('10390dd8-2f64-4177-899f-d4efe8908cce','province','GiftDelivered','{"transport_id": "bebfbd0c-abe4-4c78-a7be-87e0296d8f73", "sender_id": "befbcc44-e50f-44ec-a77e-e33dc85ee272", "recipient_id": "2c2c76f3-d465-4f82-83c4-4f6477a2ce04", "sender_name": "Myrine", "recipient_name": "Leandros", "origin_id": "45a17810-e655-45db-a6cf-8320581eef62", "origin_name": "SECRET GIFT ORIGIN", "destination_id": "10390dd8-2f64-4177-899f-d4efe8908cce", "destination_name": "Nostos", "good_key": "copper", "quantity": 1.5, "credited_quantity": 1.5, "lost_quantity": 0}','8f730778-0cf2-4602-964e-7f50c655f072');
