UPDATE players SET wanax_name='Leandros' WHERE id='a26b11bf-6251-48ac-92c6-29a09df28348'

UPDATE players SET wanax_name='Myrine' WHERE id='f3bf957f-281a-4635-8783-65cd711c8a81'

UPDATE players SET wanax_name='Dorian' WHERE id='2988c0aa-4da1-434e-84f1-93eefb70f3cc'

UPDATE map_tiles SET terrain='plains',coastal=true WHERE world_id='62671803-e59b-4796-b58b-53d6ca56f117' AND r=49 AND q BETWEEN 1 AND 7; UPDATE map_tiles SET terrain='coastal_sea' WHERE world_id='62671803-e59b-4796-b58b-53d6ca56f117' AND (q,r) IN((0,50),(1,50),(0,51));

UPDATE units SET q=1,r=49,status='positioned' WHERE id='a90c0aae-b63d-40be-92b1-891e22e21438';

UPDATE units SET q=1,r=49,status='garrison',settlement_id='df3ed4a0-1b84-408a-8129-650bc0b9b07e',support_settlement_id='df3ed4a0-1b84-408a-8129-650bc0b9b07e' WHERE world_id='62671803-e59b-4796-b58b-53d6ca56f117' AND owner_id='a26b11bf-6251-48ac-92c6-29a09df28348' AND type='spearman';

UPDATE units SET q=7,r=49,status='positioned' WHERE id='1f7f04e3-1863-4803-9c65-3d2faa0914ae';

UPDATE units SET q=7,r=49,status='garrison',settlement_id='64f5f0f2-7e86-4210-873c-5572449dd65c',support_settlement_id='64f5f0f2-7e86-4210-873c-5572449dd65c' WHERE world_id='62671803-e59b-4796-b58b-53d6ca56f117' AND owner_id='f3bf957f-281a-4635-8783-65cd711c8a81' AND type='spearman';

UPDATE units SET q=3,r=49,status='positioned' WHERE id='f3d05707-5284-4d1d-aacd-57c01c2facb1'; UPDATE founder_phase SET is_refugee=true WHERE host_unit_id='f3d05707-5284-4d1d-aacd-57c01c2facb1'; UPDATE worlds SET current_tick=100,last_tick_at=now() WHERE id='62671803-e59b-4796-b58b-53d6ca56f117';

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('62671803-e59b-4796-b58b-53d6ca56f117','a26b11bf-6251-48ac-92c6-29a09df28348','b2cc2b31-865a-4d8d-bd4e-8879534c2927') ON CONFLICT DO NOTHING

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('62671803-e59b-4796-b58b-53d6ca56f117','f3bf957f-281a-4635-8783-65cd711c8a81','a78c307a-82c5-44da-81bf-8fbff7cec562') ON CONFLICT DO NOTHING

UPDATE provinces SET coastal=true WHERE id='a78c307a-82c5-44da-81bf-8fbff7cec562'; INSERT INTO buildings(settlement_id,building_type,level,god_id) VALUES('df3ed4a0-1b84-408a-8129-650bc0b9b07e','harbour',1,NULL),('df3ed4a0-1b84-408a-8129-650bc0b9b07e','temple',3,'potnia') ON CONFLICT DO NOTHING; UPDATE player_world_records SET kharis_amount=90,kharis_rate=0,kharis_calc_tick=100 WHERE world_id='62671803-e59b-4796-b58b-53d6ca56f117' AND player_id='a26b11bf-6251-48ac-92c6-29a09df28348'; UPDATE settlement_goods SET amount=10000,rate=0,cap=1000000,calc_tick=100 WHERE settlement_id IN('df3ed4a0-1b84-408a-8129-650bc0b9b07e','64f5f0f2-7e86-4210-873c-5572449dd65c'); DELETE FROM settlement_placement WHERE settlement_id='df3ed4a0-1b84-408a-8129-650bc0b9b07e'; INSERT INTO settlement_placement(settlement_id,gubbe_ordinal,target_kind,hex_q,hex_r,good_key) VALUES('df3ed4a0-1b84-408a-8129-650bc0b9b07e',1,'hex',0,50,'fish');

INSERT INTO sea_storms(world_id,heading,created_tick) VALUES('62671803-e59b-4796-b58b-53d6ca56f117',0,100) RETURNING id

INSERT INTO sea_storm_track(storm_id,tick,slot,q,r) VALUES('e5e57d39-537e-4b03-b2ae-55a5d993bd25',100,0,0,50),('e5e57d39-537e-4b03-b2ae-55a5d993bd25',100,1,1,50),('e5e57d39-537e-4b03-b2ae-55a5d993bd25',100,2,0,51);

UPDATE map_tiles SET terrain='coastal_sea' WHERE world_id='62671803-e59b-4796-b58b-53d6ca56f117' AND q=1 AND r=51; INSERT INTO sea_storm_track(storm_id,tick,slot,q,r) VALUES('e5e57d39-537e-4b03-b2ae-55a5d993bd25',101,0,1,50),('e5e57d39-537e-4b03-b2ae-55a5d993bd25',101,1,0,51),('e5e57d39-537e-4b03-b2ae-55a5d993bd25',101,2,1,51);

INSERT INTO sea_storm_track(storm_id,tick,slot,q,r) VALUES('e5e57d39-537e-4b03-b2ae-55a5d993bd25',102,0,0,50),('e5e57d39-537e-4b03-b2ae-55a5d993bd25',102,1,1,50),('e5e57d39-537e-4b03-b2ae-55a5d993bd25',102,2,0,51);
