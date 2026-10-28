UPDATE players SET wanax_name='Leandros' WHERE id='6d41bf5c-49aa-4b49-bfe8-03ef3fc0dac8'

UPDATE players SET wanax_name='Myrine' WHERE id='a46b9bf1-8c6a-4543-9b6a-182e92a3938d'

UPDATE players SET wanax_name='Dorian' WHERE id='5e0ac839-7912-476a-a52e-8df1f62a47eb'

UPDATE map_tiles SET terrain='plains',coastal=true WHERE world_id='a11ea02c-0955-408d-83c6-d913b9a54fe2' AND r=27 AND q BETWEEN 1 AND 7; UPDATE map_tiles SET terrain='coastal_sea' WHERE world_id='a11ea02c-0955-408d-83c6-d913b9a54fe2' AND (q,r) IN((0,28),(1,28),(0,29));

UPDATE units SET q=1,r=27,status='positioned' WHERE id='eee1365b-d4f3-4226-944c-d9ca617abde2';

UPDATE units SET q=1,r=27,status='garrison',settlement_id='807020b0-84ba-4efb-94dd-d00ff1700607',support_settlement_id='807020b0-84ba-4efb-94dd-d00ff1700607' WHERE world_id='a11ea02c-0955-408d-83c6-d913b9a54fe2' AND owner_id='6d41bf5c-49aa-4b49-bfe8-03ef3fc0dac8' AND type='spearman';

UPDATE units SET q=7,r=27,status='positioned' WHERE id='95b95dfd-6ac4-4e4e-ae66-e674719b91e2';

UPDATE units SET q=7,r=27,status='garrison',settlement_id='42650b7b-c595-46f9-b230-794231b60075',support_settlement_id='42650b7b-c595-46f9-b230-794231b60075' WHERE world_id='a11ea02c-0955-408d-83c6-d913b9a54fe2' AND owner_id='a46b9bf1-8c6a-4543-9b6a-182e92a3938d' AND type='spearman';

UPDATE units SET q=3,r=27,status='positioned' WHERE id='7c20a66a-92b0-41c6-ba3e-7066d9c0987a'; UPDATE founder_phase SET is_refugee=true WHERE host_unit_id='7c20a66a-92b0-41c6-ba3e-7066d9c0987a'; UPDATE worlds SET current_tick=100,last_tick_at=now() WHERE id='a11ea02c-0955-408d-83c6-d913b9a54fe2';

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('a11ea02c-0955-408d-83c6-d913b9a54fe2','6d41bf5c-49aa-4b49-bfe8-03ef3fc0dac8','f15a7f7b-6032-4b58-8bdd-516a00833ec6') ON CONFLICT DO NOTHING

INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES('a11ea02c-0955-408d-83c6-d913b9a54fe2','a46b9bf1-8c6a-4543-9b6a-182e92a3938d','230f6938-71f9-4b7f-b529-ddeba768516c') ON CONFLICT DO NOTHING

UPDATE provinces SET coastal=true WHERE id='230f6938-71f9-4b7f-b529-ddeba768516c'; INSERT INTO buildings(settlement_id,building_type,level,god_id) VALUES('807020b0-84ba-4efb-94dd-d00ff1700607','harbour',1,NULL),('807020b0-84ba-4efb-94dd-d00ff1700607','temple',3,'potnia') ON CONFLICT DO NOTHING; UPDATE player_world_records SET kharis_amount=90,kharis_rate=0,kharis_calc_tick=100 WHERE world_id='a11ea02c-0955-408d-83c6-d913b9a54fe2' AND player_id='6d41bf5c-49aa-4b49-bfe8-03ef3fc0dac8'; UPDATE settlement_goods SET amount=10000,rate=0,cap=1000000,calc_tick=100 WHERE settlement_id IN('807020b0-84ba-4efb-94dd-d00ff1700607','42650b7b-c595-46f9-b230-794231b60075'); DELETE FROM settlement_placement WHERE settlement_id='807020b0-84ba-4efb-94dd-d00ff1700607'; INSERT INTO settlement_placement(settlement_id,gubbe_ordinal,target_kind,hex_q,hex_r,good_key) VALUES('807020b0-84ba-4efb-94dd-d00ff1700607',1,'hex',0,28,'fish');

INSERT INTO sea_storms(world_id,heading,created_tick) VALUES('a11ea02c-0955-408d-83c6-d913b9a54fe2',0,100) RETURNING id

INSERT INTO sea_storm_track(storm_id,tick,slot,q,r) VALUES('e1bfae46-ad8b-4c1e-955e-d028e115856d',100,0,0,28),('e1bfae46-ad8b-4c1e-955e-d028e115856d',100,1,1,28),('e1bfae46-ad8b-4c1e-955e-d028e115856d',100,2,0,29);

UPDATE map_tiles SET terrain='coastal_sea' WHERE world_id='a11ea02c-0955-408d-83c6-d913b9a54fe2' AND q=1 AND r=29; INSERT INTO sea_storm_track(storm_id,tick,slot,q,r) VALUES('e1bfae46-ad8b-4c1e-955e-d028e115856d',101,0,1,28),('e1bfae46-ad8b-4c1e-955e-d028e115856d',101,1,0,29),('e1bfae46-ad8b-4c1e-955e-d028e115856d',101,2,1,29);

INSERT INTO sea_storm_track(storm_id,tick,slot,q,r) VALUES('e1bfae46-ad8b-4c1e-955e-d028e115856d',102,0,0,28),('e1bfae46-ad8b-4c1e-955e-d028e115856d',102,1,1,28),('e1bfae46-ad8b-4c1e-955e-d028e115856d',102,2,0,29);
