#!/usr/bin/env python3
"""Read-only deploy verification: process provenance, static bytes, ground state."""
import hashlib,json,pathlib,subprocess,urllib.request,sys
root=pathlib.Path(__file__).resolve().parents[1]
expected=subprocess.check_output(['git','-C',str(root),'rev-parse','HEAD'],text=True).strip()
remote=r'''
import pathlib,shlex,subprocess,json,urllib.request
health=json.load(urllib.request.urlopen('http://localhost:8080/healthz'))
if not before:
 assert expected.startswith(health['commit']),health
 assert health['status']=='ok',health
vals={}
for line in pathlib.Path('/opt/poleia/.env').read_text().splitlines():
 line=line.strip()
 if line and not line.startswith('#') and '=' in line:
  k,v=line.split('=',1)
  if k.strip()=='DATABASE_URL':vals[k.strip()]=shlex.split(v)[0]
q="SELECT json_build_object('migration',(SELECT row_to_json(m) FROM schema_migrations m),'worlds',(SELECT json_agg(json_build_object('id',id,'status',status,'tick',current_tick)) FROM worlds),'players',(SELECT count(*) FROM players),'settlements',(SELECT count(*) FROM settlements),'units',(SELECT count(*) FROM units),'goods',(SELECT count(*) FROM settlement_goods),'gossip',(SELECT count(*) FROM gossip_events));"
r=subprocess.run(['psql',vals['DATABASE_URL'],'-XAt','-v','ON_ERROR_STOP=1','-c',q],capture_output=True,text=True)
if r.returncode:raise SystemExit('read-only ground query failed')
print(json.dumps({'ground':json.loads(r.stdout),'health':health,'checkout':subprocess.check_output(['git','-C','/opt/poleia','rev-parse','HEAD'],text=True).strip()}))
'''
remote='expected='+repr(expected)+'\nbefore='+repr('--before' in sys.argv)+'\n'+remote
r=subprocess.run(['ssh','root@10.0.1.92','python3','-'],input=remote,capture_output=True,text=True)
if r.returncode:raise SystemExit(r.stderr.strip() or "remote verification failed")
proof=json.loads(r.stdout)
if '--before' in sys.argv:
 (root/'docs/reviews/kult-flikar/ground-before.json').write_text(json.dumps(proof['ground'],indent=2)+'\n')
 print('Captured read-only ground baseline')
 raise SystemExit(0)
assert proof['checkout']==expected,proof
assert expected.startswith(proof['health']['commit']),proof
assert proof['health']['migration']==217 and proof['health']['status']=='ok',proof
assert proof['ground']['migration']=={'version':217,'dirty':False},proof
before=json.loads((root/'docs/reviews/kult-flikar/ground-before.json').read_text())
assert {(x['id'],x['status']) for x in before['worlds']}=={(x['id'],x['status']) for x in proof['ground']['worlds']},proof
for k in ['players','settlements','units','goods']:assert before[k]==proof['ground'][k],(k,before[k],proof['ground'][k])
proof['static']={}
files=subprocess.check_output(['git','-C',str(root),'diff','--name-only','b1080116','HEAD','--','web/static'],text=True).splitlines()
for f in files:
 if not (root/f).is_file():continue
 url='http://10.0.1.92:8080/static/'+f.removeprefix('web/static/')
 actual=hashlib.sha256(urllib.request.urlopen(url).read()).hexdigest();want=hashlib.sha256((root/f).read_bytes()).hexdigest()
 assert actual==want,f
 proof['static'][f]=actual
world=next(w['id'] for w in proof['ground']['worlds'] if w['status']=='active')
import urllib.error
try:
 urllib.request.urlopen('http://10.0.1.92:8080/api/v1/worlds/'+world+'/cult')
 raise AssertionError('cult accepted anonymous request')
except urllib.error.HTTPError as error:
 assert error.code==401,error.code
 proof['cult_anonymous_status']=error.code
proof['expected']=expected;proof['before']=before
(root/'docs/reviews/kult-flikar/deploy-proof.json').write_text(json.dumps(proof,indent=2)+'\n')
print('PASS live process hash, clean217, preserved ground state,',len(proof['static']),'byte-exact static assets')
