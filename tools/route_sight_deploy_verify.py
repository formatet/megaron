#!/usr/bin/env python3
"""Read-only provenance, ground-state and unchanged asset verification."""
from pathlib import Path
import hashlib,json,subprocess,sys,urllib.request
root=Path(__file__).resolve().parents[1];out=root/'docs/reviews/rutthex-synminne'
remote=r'''
import pathlib,shlex,subprocess,json,urllib.request
health=json.load(urllib.request.urlopen('http://localhost:8080/healthz'))
for line in pathlib.Path('/opt/poleia/.env').read_text().splitlines():
 if line.startswith('DATABASE_URL='):dsn=shlex.split(line.split('=',1)[1])[0]
q="SELECT json_build_object('migration',(SELECT row_to_json(m) FROM schema_migrations m),'worlds',(SELECT json_agg(json_build_object('id',id,'status',status,'tick',current_tick)) FROM worlds),'players',(SELECT count(*) FROM players),'settlements',(SELECT count(*) FROM settlements),'units',(SELECT count(*) FROM units),'goods',(SELECT count(*) FROM settlement_goods),'unit_positions',(SELECT md5(jsonb_agg(jsonb_build_object('id',id,'status',status,'q',q,'r',r,'owner',owner_id,'support',support_settlement_id,'settlement',settlement_id) ORDER BY id)::text) FROM units))"
s=subprocess.check_output(['psql',dsn,'-XAt','-v','ON_ERROR_STOP=1','-c',q],text=True)
print(json.dumps({'health':health,'ground':json.loads(s),'checkout':subprocess.check_output(['git','-C','/opt/poleia','rev-parse','HEAD'],text=True).strip()}))
'''
result=json.loads(subprocess.check_output(['ssh','root@10.0.1.92','python3','-'],input=remote,text=True))
if '--before' in sys.argv:
 (out/'ground-before.json').write_text(json.dumps(result,indent=2)+'\n');print('Saved read-only ground baseline');raise SystemExit
expected=subprocess.check_output(['git','-C',str(root),'rev-parse','HEAD'],text=True).strip()
assert result['checkout']==expected and expected.startswith(result['health']['commit']),result
assert result['health']['status']=='ok' and result['health']['migration']==217,result
assert result['ground']['migration']=={'version':217,'dirty':False},result
before=json.loads((out/'ground-before.json').read_text())
for k in ['players','settlements','units','goods','unit_positions']:assert result['ground'][k]==before['ground'][k],k
assert {(w['id'],w['status']) for w in result['ground']['worlds']}=={(w['id'],w['status']) for w in before['ground']['worlds']}
result['assets']={}
for f in ['web/static/megaron.css','web/static/js/megaron/render/map.js','web/static/js/megaron/render/routes.js','web/static/codex/marching.md']:
 local=hashlib.sha256((root/f).read_bytes()).hexdigest()
 actual=hashlib.sha256(urllib.request.urlopen('http://10.0.1.92:8080/static/'+f.removeprefix('web/static/')).read()).hexdigest()
 assert actual==local,f
 result['assets'][f]=actual
(out/'deploy-proof.json').write_text(json.dumps(result,indent=2)+'\n')
print('PASS new process',result['health']['commit'],'clean217, same ground and four byte-exact assets')
