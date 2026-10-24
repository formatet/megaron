UPDATE players SET wanax_name='Leandros' WHERE id='6bf5ecc1-1496-4181-af7f-214170a251cb';

UPDATE players SET wanax_name='Myrine' WHERE id='e2e14a4d-8529-45e3-9e29-5245bae8b036';

UPDATE players SET wanax_name='Dorian' WHERE id='61922951-ea03-480d-a085-4c88b74c1d98';

UPDATE players SET wanax_name='Thestor' WHERE id='fb65fc78-1293-4f65-aae2-30e1bfc6ba70';

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('e87393af-f9a0-49e6-b144-3a353933cd5b','6bf5ecc1-1496-4181-af7f-214170a251cb','24d13822-15f1-48b2-b1d2-74c26b56ef31') ON CONFLICT DO NOTHING;

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('e87393af-f9a0-49e6-b144-3a353933cd5b','6bf5ecc1-1496-4181-af7f-214170a251cb','e510d9fe-f9ce-4828-ac4d-a6b510e7768c') ON CONFLICT DO NOTHING;

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('e87393af-f9a0-49e6-b144-3a353933cd5b','6bf5ecc1-1496-4181-af7f-214170a251cb','d867eaf6-5157-4ddd-9fc8-30ada39522cd') ON CONFLICT DO NOTHING;

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('e87393af-f9a0-49e6-b144-3a353933cd5b','e2e14a4d-8529-45e3-9e29-5245bae8b036','50c3d129-6d3b-4028-bed2-54877397c1bd') ON CONFLICT DO NOTHING;

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('e87393af-f9a0-49e6-b144-3a353933cd5b','e2e14a4d-8529-45e3-9e29-5245bae8b036','e510d9fe-f9ce-4828-ac4d-a6b510e7768c') ON CONFLICT DO NOTHING;

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('e87393af-f9a0-49e6-b144-3a353933cd5b','e2e14a4d-8529-45e3-9e29-5245bae8b036','d867eaf6-5157-4ddd-9fc8-30ada39522cd') ON CONFLICT DO NOTHING;

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('e87393af-f9a0-49e6-b144-3a353933cd5b','61922951-ea03-480d-a085-4c88b74c1d98','50c3d129-6d3b-4028-bed2-54877397c1bd') ON CONFLICT DO NOTHING;

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('e87393af-f9a0-49e6-b144-3a353933cd5b','61922951-ea03-480d-a085-4c88b74c1d98','24d13822-15f1-48b2-b1d2-74c26b56ef31') ON CONFLICT DO NOTHING;

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('e87393af-f9a0-49e6-b144-3a353933cd5b','61922951-ea03-480d-a085-4c88b74c1d98','d867eaf6-5157-4ddd-9fc8-30ada39522cd') ON CONFLICT DO NOTHING;

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('e87393af-f9a0-49e6-b144-3a353933cd5b','fb65fc78-1293-4f65-aae2-30e1bfc6ba70','50c3d129-6d3b-4028-bed2-54877397c1bd') ON CONFLICT DO NOTHING;

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('e87393af-f9a0-49e6-b144-3a353933cd5b','fb65fc78-1293-4f65-aae2-30e1bfc6ba70','24d13822-15f1-48b2-b1d2-74c26b56ef31') ON CONFLICT DO NOTHING;

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('e87393af-f9a0-49e6-b144-3a353933cd5b','fb65fc78-1293-4f65-aae2-30e1bfc6ba70','e510d9fe-f9ce-4828-ac4d-a6b510e7768c') ON CONFLICT DO NOTHING;

UPDATE worlds SET current_tick=100,last_tick_at=now() WHERE id='e87393af-f9a0-49e6-b144-3a353933cd5b'; UPDATE settlement_goods SET amount=10000,rate=0,calc_tick=100 WHERE settlement_id IN('5837d9e8-9b62-42c6-9500-9a709ce72b0d','8e4aea09-1975-49df-bc47-19ffb7b7e90d','db9f8342-a957-42ab-b456-b80a07d5953d','88cb45fc-7cac-4f6a-a96f-0221f55ae9c0') AND good_key IN('grain','silver','copper');

UPDATE scheduled_events SET due_tick=100,process_after=now() WHERE world_id='e87393af-f9a0-49e6-b144-3a353933cd5b' AND event_type='MessengerArrival' AND payload->>'messenger_id' IN('e08fc602-4bfe-402c-a465-225e9426bbe2','e2263e97-b892-419e-8b95-97e62b152ae3','d4cd4326-0f02-40ef-8818-c50fc55a4e80');

INSERT INTO events(stream_id,stream_type,event_type,payload,world_id) VALUES('5837d9e8-9b62-42c6-9500-9a709ce72b0d','province','GiftDelivered','{"transport_id": "fa02260b-d5bd-4cac-936d-074bd2c7d101", "sender_id": "e2e14a4d-8529-45e3-9e29-5245bae8b036", "recipient_id": "6bf5ecc1-1496-4181-af7f-214170a251cb", "sender_name": "Myrine", "recipient_name": "Leandros", "origin_id": "8e4aea09-1975-49df-bc47-19ffb7b7e90d", "origin_name": "SECRET GIFT ORIGIN", "destination_id": "5837d9e8-9b62-42c6-9500-9a709ce72b0d", "destination_name": "Nostos", "good_key": "copper", "quantity": 1.5, "credited_quantity": 1.5, "lost_quantity": 0}','e87393af-f9a0-49e6-b144-3a353933cd5b');
