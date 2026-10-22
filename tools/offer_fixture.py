#!/usr/bin/env python3
"""Own temple fixture in the active isolated world; archived worlds reject orders."""
import json,pathlib,urllib.error,uuid
from fardvag_fixture import api,sql,PW
ROOT=pathlib.Path(__file__).resolve().parents[1];OUT=ROOT/'docs/reviews/offer-en-knapp'
username='OfferNestor'+uuid.uuid4().hex[:5]
token=api('auth/register',data={'username':username,'email':username+'@example.test','password':PW})['access_token'];owner=api('auth/me',token)['id']
world=sql("SELECT id FROM worlds WHERE status='active' ORDER BY created_at DESC LIMIT 1").splitlines()[0]
api(f'worlds/{world}/join',token,{})
q,r=map(int,sql(f"SELECT q,r FROM map_tiles m WHERE world_id='{world}' AND terrain='plains' AND q>5 AND r>5 AND NOT EXISTS(SELECT 1 FROM provinces p WHERE p.world_id=m.world_id AND GREATEST(abs(p.map_q-q),abs(p.map_r-r),abs(p.map_q+p.map_r-q-r))<4) ORDER BY q,r LIMIT 1").split('|'))
pid,sid=str(uuid.uuid4()),str(uuid.uuid4())
sql(f"""BEGIN;
INSERT INTO provinces(id,world_id,map_q,map_r,terrain_type) VALUES('{pid}','{world}',{q},{r},'plains');
INSERT INTO settlements(id,world_id,province_id,name,culture_id,owner_id,control_type,is_capital,population) VALUES('{sid}','{world}','{pid}','Mycenae','minoan','{owner}','capital',true,1200);
UPDATE players SET wanax_name='Nestor of Mycenae' WHERE id='{owner}';
UPDATE player_world_records SET kharis_amount=100,kharis_rate=0,kharis_cap=100,kharis_calc_tick=current_world_tick() WHERE player_id='{owner}' AND world_id='{world}';
INSERT INTO buildings(settlement_id,building_type,level,god_id) VALUES('{sid}','temple',1,'potnia');
INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) SELECT '{sid}',g,2000,0,5000,current_world_tick() FROM unnest(ARRAY['oil','wine','grain']) g;
COMMIT;""")
d=api(f'worlds/{world}/provinces/{pid}',token);prayers=d['settlement']['available_prayers'];assert len(prayers)==3 and all(p['affordable'] for p in prayers),prayers
s={'world':world,'province':pid,'settlement':sid,'viewer':owner,'username':username,'name':'Mycenae','prayer_ids':[p['id'] for p in prayers],'fixture':'own freshly registered/joined test player; explicit SQL temple/city/stock/kharis setup in active isolated world, actual API/FOW; not natural temple founding/production; Offer action uses native web/API with no SQL writes'}
(OUT/'scenario.json').write_text(json.dumps(s,indent=2)+'\n');(OUT/'fixture-api.json').write_text(json.dumps(d,indent=2)+'\n')
print('own temple fixture created; three prayers affordable',world,username)
