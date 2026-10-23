#!/usr/bin/env python3
"""Real acceptance map/CLI/browser river view with explicit, owned sight/claim fixtures.
No reset/reseed. Refuses to overwrite existing geographic claims. Cleans its own
unit and claim in finally. Does not prove natural arrival or river navigation.
"""
import json,os,pathlib,subprocess,tempfile,urllib.request,urllib.error
ROOT=pathlib.Path(__file__).resolve().parents[1]
OUT=ROOT/'docs/reviews/flod-verifiering'
BASE='http://localhost:8097'
CONFIG=pathlib.Path('/tmp/megaron-river23.json')
def api(path,token=None,data=None):
 r=urllib.request.Request(BASE+'/api/v1/'+path,data=None if data is None else json.dumps(data).encode(),headers={'Content-Type':'application/json',**({'Authorization':'Bearer '+token} if token else {})})
 return json.load(urllib.request.urlopen(r,timeout=30))
def sql(query):return subprocess.check_output([str(ROOT/'tools/acceptance.sh'),'psql',query],text=True).strip()
def main():
 OUT.mkdir(parents=True,exist_ok=True)
 world=api('worlds')[0]['id']
 user='RiverProof23'
 try:token=api('auth/login',data={'username_or_email':user,'password':'acceptance-pw-123'})['access_token']
 except urllib.error.HTTPError as e:
  if e.code!=401:raise
  token=api('auth/register',data={'username':user,'password':'acceptance-pw-123'})['access_token']
  api('worlds/'+world+'/join',token,{})
 pid=api('auth/me',token)['id']
 with os.fdopen(os.open(CONFIG,os.O_WRONLY|os.O_CREAT|os.O_TRUNC,0o600),'w') as f:
  json.dump(dict(server=BASE,token=token,world_id=world,username=user),f)
 CONFIG.chmod(0o600)
 existing_unit=sql(f"SELECT id FROM units WHERE world_id='{world}' AND owner_id='{pid}' AND name='RiverProof23 sight fixture'")
 if not existing_unit:
  ford=json.loads(sql(f"SELECT row_to_json(t) FROM (SELECT q,r FROM map_tiles WHERE world_id='{world}' AND terrain='river_ford' LIMIT 1)t"))
  sql(f"INSERT INTO units(world_id,owner_id,type,category,size,status,q,r,name) VALUES('{world}','{pid}','spearman','land',100,'positioned',{ford['q']},{ford['r']},'RiverProof23 sight fixture')")
 health=json.load(urllib.request.urlopen(BASE+'/healthz'));(OUT/'health.json').write_text(json.dumps(health))
 tiles=api('worlds/'+world+'/map',token)
 river=next(t for t in tiles if t.get('terrain')=='river' and t.get('place_id'))
 aq,ar=map(int,river['place_id'].split(','))
 existing=sql(f"SELECT count(*) FROM place_names WHERE world_id='{world}' AND kind='river' AND anchor_q={aq} AND anchor_r={ar}")
 assert existing=='0','refusing to change an existing claim'
 unit=sql(f"SELECT id FROM units WHERE world_id='{world}' AND owner_id='{pid}' AND name='RiverProof23 sight fixture'").splitlines()
 assert len(unit)==1
 try:
  sql(f"INSERT INTO place_names(world_id,kind,anchor_q,anchor_r,claimed_by,claimed_tick) SELECT '{world}','river',{aq},{ar},'{pid}',current_tick FROM worlds WHERE id='{world}'")
  api(f'worlds/{world}/places/river/{aq},{ar}/name',token,{'name':'Riverproof Thaleia'})
  tiles=api('worlds/'+world+'/map',token)
  known=[t for t in tiles if t.get('place_id')==river['place_id'] and t.get('tier')!='fog']
  assert known and all(t.get('river_name')=='Riverproof Thaleia' for t in known)
  assert all(not t.get('river_name') and not t.get('place_name') for t in tiles if t.get('tier')=='fog')
  (OUT/'named-map.json').write_text(json.dumps(tiles,indent=1))
  env=dict(os.environ,PANEL_USER='RiverProof23',PANEL_HOST_USER='BrowserHost23')
  p=subprocess.run(['python',str(ROOT/'tools/browser_parity.py'),'--surfaces','map-river','river','--out',str(OUT/'browser')],env=env,text=True,capture_output=True)
  (OUT/'browser.log').write_text(p.stdout+p.stderr);assert p.returncode==0,p.stdout+p.stderr
  with tempfile.TemporaryDirectory(prefix='river-view-') as tmp:
   binary=pathlib.Path(tmp)/'keryx';subprocess.run(['go','build','-o',str(binary),'./cmd/keryx'],cwd=ROOT/'server',check=True)
   env=dict(os.environ,POLEIA_CONFIG=str(CONFIG));env.pop('POLEIA_TOKEN',None);env.pop('POLEIA_SERVER',None)
   for label,args in [('map',['map','--radius','100','--json']),('places',['places'])]:
    p=subprocess.run([str(binary),*args],env=env,text=True,capture_output=True);(OUT/(label+'.txt')).write_text(p.stdout+p.stderr);assert p.returncode==0,p.stderr
    assert 'Riverproof Thaleia' in p.stdout
  (OUT/'facts.json').write_text(json.dumps({'health':health,'world':world,'player':pid,'river_anchor':[aq,ar],'known_hexes':len(known),'fog_name_leak':False,'fixture':'SQL-owned scout + first-reach claim; public naming API, real map API/CLI/browser; not natural progression'},indent=2))
 finally:
  sql(f"DELETE FROM place_names WHERE world_id='{world}' AND kind='river' AND anchor_q={aq} AND anchor_r={ar} AND claimed_by='{pid}'")
  sql(f"DELETE FROM units WHERE id='{unit[0]}' AND owner_id='{pid}'")
  (OUT/'cleanup.txt').write_text('Own first-reach claim and positioned sight unit removed. No reset/reseed.\n')
if __name__=='__main__':main()
