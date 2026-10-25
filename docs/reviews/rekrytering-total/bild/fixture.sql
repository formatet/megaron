UPDATE provinces SET coastal=true WHERE id='81c5d6b5-be30-45c3-8463-0cf91cc4fb6a'; INSERT INTO buildings(settlement_id,building_type,level) VALUES('23396245-876d-4f3a-8d35-c92072e7159d','barracks',1),('23396245-876d-4f3a-8d35-c92072e7159d','harbour',1),('23396245-876d-4f3a-8d35-c92072e7159d','shipyard',1) ON CONFLICT DO NOTHING; UPDATE settlement_goods SET amount=CASE good_key WHEN 'timber' THEN 0 WHEN 'cedar' THEN 749 ELSE 10000 END,rate=0,cap=1000000,calc_tick=current_world_tick() WHERE settlement_id='23396245-876d-4f3a-8d35-c92072e7159d';

UPDATE settlement_goods SET amount=CASE good_key WHEN 'timber' THEN 10 WHEN 'cedar' THEN 0 ELSE amount END,rate=0,calc_tick=current_world_tick() WHERE settlement_id='23396245-876d-4f3a-8d35-c92072e7159d';

UPDATE settlement_goods SET amount=CASE good_key WHEN 'timber' THEN 0 WHEN 'cedar' THEN 749 ELSE amount END,rate=0,calc_tick=current_world_tick() WHERE settlement_id='23396245-876d-4f3a-8d35-c92072e7159d';
