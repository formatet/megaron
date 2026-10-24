UPDATE players SET wanax_name='Leandros' WHERE id='3494b440-9ede-43de-9b80-8b08ebcee169';

UPDATE players SET wanax_name='Myrine' WHERE id='faa69b12-8646-48f7-88e7-9bc5325c154b';

UPDATE players SET wanax_name='Dorian' WHERE id='93da265c-2ae6-4869-b28a-fc02320e4535';

UPDATE players SET wanax_name='Thestor' WHERE id='d756b50e-85a3-45ed-b05d-860853d0716a';

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('6a4dbce8-bd57-4720-af36-4fa0db55ba2c','3494b440-9ede-43de-9b80-8b08ebcee169','b01bb0ab-5e27-45b1-97ad-7f837db9bc98') ON CONFLICT DO NOTHING;

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('6a4dbce8-bd57-4720-af36-4fa0db55ba2c','3494b440-9ede-43de-9b80-8b08ebcee169','66890ea0-565d-43ed-8461-4eebd644d7be') ON CONFLICT DO NOTHING;

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('6a4dbce8-bd57-4720-af36-4fa0db55ba2c','3494b440-9ede-43de-9b80-8b08ebcee169','aeeab989-3e73-45c6-8550-7a8733b0c1a0') ON CONFLICT DO NOTHING;

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('6a4dbce8-bd57-4720-af36-4fa0db55ba2c','faa69b12-8646-48f7-88e7-9bc5325c154b','86e3a25c-4221-4df6-b393-de2653cf511b') ON CONFLICT DO NOTHING;

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('6a4dbce8-bd57-4720-af36-4fa0db55ba2c','faa69b12-8646-48f7-88e7-9bc5325c154b','66890ea0-565d-43ed-8461-4eebd644d7be') ON CONFLICT DO NOTHING;

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('6a4dbce8-bd57-4720-af36-4fa0db55ba2c','faa69b12-8646-48f7-88e7-9bc5325c154b','aeeab989-3e73-45c6-8550-7a8733b0c1a0') ON CONFLICT DO NOTHING;

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('6a4dbce8-bd57-4720-af36-4fa0db55ba2c','93da265c-2ae6-4869-b28a-fc02320e4535','86e3a25c-4221-4df6-b393-de2653cf511b') ON CONFLICT DO NOTHING;

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('6a4dbce8-bd57-4720-af36-4fa0db55ba2c','93da265c-2ae6-4869-b28a-fc02320e4535','b01bb0ab-5e27-45b1-97ad-7f837db9bc98') ON CONFLICT DO NOTHING;

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('6a4dbce8-bd57-4720-af36-4fa0db55ba2c','93da265c-2ae6-4869-b28a-fc02320e4535','aeeab989-3e73-45c6-8550-7a8733b0c1a0') ON CONFLICT DO NOTHING;

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('6a4dbce8-bd57-4720-af36-4fa0db55ba2c','d756b50e-85a3-45ed-b05d-860853d0716a','86e3a25c-4221-4df6-b393-de2653cf511b') ON CONFLICT DO NOTHING;

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('6a4dbce8-bd57-4720-af36-4fa0db55ba2c','d756b50e-85a3-45ed-b05d-860853d0716a','b01bb0ab-5e27-45b1-97ad-7f837db9bc98') ON CONFLICT DO NOTHING;

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('6a4dbce8-bd57-4720-af36-4fa0db55ba2c','d756b50e-85a3-45ed-b05d-860853d0716a','66890ea0-565d-43ed-8461-4eebd644d7be') ON CONFLICT DO NOTHING;

UPDATE worlds SET current_tick=100,last_tick_at=now() WHERE id='6a4dbce8-bd57-4720-af36-4fa0db55ba2c'; UPDATE settlement_goods SET amount=10000,rate=0,calc_tick=100 WHERE settlement_id IN('4adab46b-565f-44a5-995d-4a9edfe5e5ec','b655cd27-68ff-48f6-83c2-6d89cd97bdea','98a35b97-4389-43b6-8781-7adb73f0a889','5fa44f44-7f19-4822-85ab-5531f85f9e6e') AND good_key IN('grain','silver','copper');

UPDATE scheduled_events SET due_tick=100,process_after=now() WHERE world_id='6a4dbce8-bd57-4720-af36-4fa0db55ba2c' AND event_type='MessengerArrival' AND payload->>'messenger_id' IN('b72930bd-3a21-479d-a68b-80e2bf3a4edc','d983482e-d44f-4698-9a2c-9ffbd1efc8d1','d96c47d7-f59f-4996-b551-a66614e55904');
