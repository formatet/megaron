#!/usr/bin/env python3
"""Explicit archived temple layout fixture; only creates its own isolated rows."""
import json,pathlib,uuid
from fardvag_fixture import api,credentials,sql,literal
ROOT=pathlib.Path(__file__).resolve().parents[1];OUT=ROOT/'docs/reviews/offer-en-knapp'
token,owner=credentials();world=str(uuid.uuid4())
sql(f"INSERT INTO worlds(id,name,status,state,current_tick,map_width,map_height) VALUES('{world}','Offer {world[:8]}','archived','active',128,13,11)")
sql(f"INSERT INTO map_tiles(world_id,q,r,terrain,fertility,mineral) SELECT '{world}',q,r,'plains',.7,.5 FROM generate_series(0,12) q CROSS JOIN generate_series(0,10) r")
pid=sql(f"INSERT INTO provinces(world_id,map_q,map_r,terrain_type) VALUES('{world}',6,5,'plains') RETURNING id").splitlines()[0]
sid=sql(f"INSERT INTO settlements(world_id,province_id,name,culture_id,owner_id,control_type,is_capital,population) VALUES('{world}','{pid}','Mycenae','minoan','{owner}','capital',true,1200) RETURNING id").splitlines()[0]
sql(f"INSERT INTO player_world_records(player_id,world_id,kharis_amount,kharis_rate,kharis_cap,kharis_calc_tick) VALUES('{owner}','{world}',100,0,100,128)")
sql(f"INSERT INTO buildings(settlement_id,building_type,level) VALUES('{sid}','temple',1)")
sql(f"INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) SELECT '{sid}',g,200,0,500,128 FROM unnest(ARRAY['oil','wine','grain']) g ON CONFLICT(settlement_id,good_key) DO UPDATE SET amount=200,rate=0,cap=500,calc_tick=128")
d=api(f'worlds/{world}/provinces/{pid}',token);prayers=d['settlement']['available_prayers'];assert len(prayers)==3 and all(p['affordable'] for p in prayers),prayers
s={'world':world,'province':pid,'settlement':sid,'viewer':owner,'name':'Mycenae','prayer_ids':[p['id'] for p in prayers],'fixture':'explicit SQL setup of a private archived temple world, actual API/FOW and stock; not natural temple founding/production; player action uses native web/API without SQL writes'}
(OUT/'scenario.json').write_text(json.dumps(s,indent=2)+'\n');(OUT/'fixture-api.json').write_text(json.dumps(d,indent=2)+'\n')
print('archived temple fixture created; all three prayers affordable',world)
