#!/usr/bin/env python3
"""Register/join a real Host and read its shared-floor Kharis via API and CLI."""
import json,os,pathlib,re,subprocess,tempfile,urllib.request,uuid
from fardvag_fixture import api,BASE,PW
ROOT=pathlib.Path(__file__).resolve().parents[1];OUT=ROOT/'docs/reviews/kharis-golv-vard'
WORLD='3b5fb9cd-8c06-4685-88df-b162c9acff99'
health=json.load(urllib.request.urlopen(BASE+'/healthz'));assert health['migration']==174,health
username='HostFloor'+uuid.uuid4().hex[:8]
token=api('auth/register',data={'username':username,'password':PW})['access_token']
joined=api('worlds/'+WORLD+'/join',token,data={})
profile=api('worlds/'+WORLD+'/me',token)
floor=float(re.search(r'const kharisFloor = ([0-9.]+)',(ROOT/'server/internal/kharis/tick.go').read_text())[1])
assert profile['kharis']==floor and profile['founded_tick'] is None and profile['cities']==0,profile
oldtoken=api('auth/login',data={'username_or_email':'ParityNomad','password':PW})['access_token']
repaired=api('worlds/'+WORLD+'/me',oldtoken)
assert repaired['kharis']==profile['kharis'],repaired
with tempfile.TemporaryDirectory(prefix='host-floor-proof-') as tmp:
 tmp=pathlib.Path(tmp);subprocess.run(['go','build','-o',str(tmp/'keryx'),'./cmd/keryx'],cwd=ROOT/'server',check=True)
 config=tmp/'config.json';config.write_text(json.dumps({'server':BASE,'world_id':WORLD,'token':token}));config.chmod(0o600)
 env=dict(os.environ,POLEIA_CONFIG=str(config));env.pop('POLEIA_SERVER',None);env.pop('POLEIA_TOKEN',None)
 text=subprocess.check_output([str(tmp/'keryx'),'me'],env=env,text=True)
 raw=json.loads(subprocess.check_output([str(tmp/'keryx'),'me','--json'],env=env,text=True))
 assert raw['kharis']==profile['kharis'] and f"Kharis: {profile['kharis']:.1f}" in text
 (OUT/'new-host-cli.txt').write_text(text)
 status=subprocess.check_output([str(tmp/'keryx'),'status'],env=env,text=True);(OUT/'new-host-status.txt').write_text(status)
assert 'Host' in status or 'host' in status,status
(OUT/'verification.json').write_text(json.dumps({'health':health,'world':WORLD,'username':username,'join':joined,'floor_from_source':floor,'new_host':profile,'repaired_existing_host':repaired,'cli_json':raw,'sql_writes':False,'payload_overrides':False,'note':'Existing Eumedes read 0.0 in prior profile proof; migration174 now reads same floor as newly joined Host. Status keeps existing Host output without adding a numeric Kharis row.'},indent=2)+'\n')
print('real register+join+API me+CLI me/json/status; existing Host repaired; no SQL writes; green')
