UPDATE players SET wanax_name='Leandros' WHERE id='b8341a35-8010-443d-a658-822380905f7d';

UPDATE players SET wanax_name='Myrine' WHERE id='cd4a9b14-d5bb-4384-a578-b6d12c01efdf';

UPDATE players SET wanax_name='Dorian' WHERE id='81df0155-e8ef-4ead-bcc2-3601dfd261d5';

UPDATE players SET wanax_name='Thestor' WHERE id='85d25d7c-914f-4215-96f7-cb0ef4d858ab';

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('9336daa6-aacc-4cde-9891-d6c86a1f554f','b8341a35-8010-443d-a658-822380905f7d','1a9b63b2-2fec-4923-9d72-886d0ab0cb95') ON CONFLICT DO NOTHING;

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('9336daa6-aacc-4cde-9891-d6c86a1f554f','b8341a35-8010-443d-a658-822380905f7d','2f63856e-42a2-4cda-912d-9c7ea8e45b3c') ON CONFLICT DO NOTHING;

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('9336daa6-aacc-4cde-9891-d6c86a1f554f','b8341a35-8010-443d-a658-822380905f7d','a2e8e25d-8a9b-4468-bde0-3a50ef61bc15') ON CONFLICT DO NOTHING;

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('9336daa6-aacc-4cde-9891-d6c86a1f554f','cd4a9b14-d5bb-4384-a578-b6d12c01efdf','85d727be-d4d5-45cf-8394-5bb5bde35c4d') ON CONFLICT DO NOTHING;

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('9336daa6-aacc-4cde-9891-d6c86a1f554f','cd4a9b14-d5bb-4384-a578-b6d12c01efdf','2f63856e-42a2-4cda-912d-9c7ea8e45b3c') ON CONFLICT DO NOTHING;

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('9336daa6-aacc-4cde-9891-d6c86a1f554f','cd4a9b14-d5bb-4384-a578-b6d12c01efdf','a2e8e25d-8a9b-4468-bde0-3a50ef61bc15') ON CONFLICT DO NOTHING;

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('9336daa6-aacc-4cde-9891-d6c86a1f554f','81df0155-e8ef-4ead-bcc2-3601dfd261d5','85d727be-d4d5-45cf-8394-5bb5bde35c4d') ON CONFLICT DO NOTHING;

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('9336daa6-aacc-4cde-9891-d6c86a1f554f','81df0155-e8ef-4ead-bcc2-3601dfd261d5','1a9b63b2-2fec-4923-9d72-886d0ab0cb95') ON CONFLICT DO NOTHING;

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('9336daa6-aacc-4cde-9891-d6c86a1f554f','81df0155-e8ef-4ead-bcc2-3601dfd261d5','a2e8e25d-8a9b-4468-bde0-3a50ef61bc15') ON CONFLICT DO NOTHING;

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('9336daa6-aacc-4cde-9891-d6c86a1f554f','85d25d7c-914f-4215-96f7-cb0ef4d858ab','85d727be-d4d5-45cf-8394-5bb5bde35c4d') ON CONFLICT DO NOTHING;

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('9336daa6-aacc-4cde-9891-d6c86a1f554f','85d25d7c-914f-4215-96f7-cb0ef4d858ab','1a9b63b2-2fec-4923-9d72-886d0ab0cb95') ON CONFLICT DO NOTHING;

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('9336daa6-aacc-4cde-9891-d6c86a1f554f','85d25d7c-914f-4215-96f7-cb0ef4d858ab','2f63856e-42a2-4cda-912d-9c7ea8e45b3c') ON CONFLICT DO NOTHING;

UPDATE worlds SET current_tick=100,last_tick_at=now() WHERE id='9336daa6-aacc-4cde-9891-d6c86a1f554f'; UPDATE settlement_goods SET amount=10000,rate=0,calc_tick=100 WHERE settlement_id IN('59446fb0-aa38-4cc3-b796-1d097c05c59f','e6d785d8-1fa7-4746-8446-2f6659fb80af','e321929f-fbb5-444a-aba7-fbd22a5fcf97','3ad8c392-0773-469e-b94a-785dc2c6725c') AND good_key IN('grain','silver','copper');

UPDATE scheduled_events SET due_tick=100,process_after=now() WHERE world_id='9336daa6-aacc-4cde-9891-d6c86a1f554f' AND event_type='MessengerArrival' AND payload->>'messenger_id' IN('0bad56a4-3e1e-48a0-a121-1ea799a8838a','2ee1322c-41c6-4f19-a122-ed939250580d','27b0c7ed-01f9-4bfc-bbde-d416a39f29b4');
