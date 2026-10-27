UPDATE map_tiles SET terrain='plains',coastal=false WHERE world_id='76887b11-b130-42b5-b228-d0afee5fde80';

INSERT INTO buildings(settlement_id,building_type,level,god_id) VALUES('6810d8d1-7129-461c-ae30-6151d7ae2f0b','temple',2,'potnia') ON CONFLICT DO NOTHING;

UPDATE player_world_records SET kharis_amount=4,kharis_rate=0,kharis_calc_tick=current_world_tick() WHERE player_id='64105729-6ea0-4425-a9e3-3bbd49b290dd' AND world_id='76887b11-b130-42b5-b228-d0afee5fde80';

UPDATE player_world_records SET kharis_amount=70,kharis_rate=0,kharis_calc_tick=current_world_tick() WHERE player_id='64105729-6ea0-4425-a9e3-3bbd49b290dd' AND world_id='76887b11-b130-42b5-b228-d0afee5fde80';

UPDATE player_world_records SET kharis_amount=4,kharis_rate=0,kharis_calc_tick=current_world_tick() WHERE player_id='64105729-6ea0-4425-a9e3-3bbd49b290dd' AND world_id='76887b11-b130-42b5-b228-d0afee5fde80';

UPDATE player_world_records SET kharis_amount=70,kharis_rate=0,kharis_calc_tick=current_world_tick() WHERE player_id='64105729-6ea0-4425-a9e3-3bbd49b290dd' AND world_id='76887b11-b130-42b5-b228-d0afee5fde80';
