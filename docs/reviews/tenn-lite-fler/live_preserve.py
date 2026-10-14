#!/usr/bin/env python3
"""Strictly read-only live provenance and old-event/wallet snapshots."""
import json,pathlib,subprocess,sys,urllib.request
here=pathlib.Path(__file__).resolve().parent
phase=sys.argv[1];assert phase in ('before','after')
before=json.loads((here/'live-before.json').read_text()) if phase=='after' else None
cutoff=str(before['snapshot']['cutoff']) if before else '(SELECT COALESCE(MAX(id),0) FROM events)'
sql="""BEGIN READ ONLY;
SELECT json_build_object(
 'migration',(SELECT json_build_object('version',version,'dirty',dirty) FROM schema_migrations),
 'world',(SELECT json_build_object('id',id,'tick',current_tick,'last_tick_at',last_tick_at) FROM worlds WHERE status='active'),
 'cutoff',CUTOFF,
 'old_event_hash',(SELECT md5(string_agg(id::text||payload::text,',' ORDER BY id)) FROM events WHERE id<=CUTOFF),
 'founders',(SELECT COALESCE(json_agg(json_build_object('id',id,'silver_amount',silver_amount,'silver_rate',silver_rate,'calc_tick',calc_tick) ORDER BY id),'[]') FROM founder_phase WHERE active),
 'map_hash',(SELECT md5(string_agg(row_to_json(mt)::text,',' ORDER BY world_id,q,r)) FROM map_tiles mt)
); COMMIT;""".replace('CUTOFF',cutoff)
result=subprocess.run(['ssh','root@10.0.1.92','runuser -u postgres -- psql -d poleia -X -qAt -v ON_ERROR_STOP=1'],input=sql,text=True,stdout=subprocess.PIPE,stderr=subprocess.PIPE)
assert result.returncode==0,result.stderr
snapshot=json.loads(result.stdout.strip())
health=json.load(urllib.request.urlopen('http://10.0.1.92:8080/healthz'))
(here/('live-'+phase+'.json')).write_text(json.dumps({'health':health,'snapshot':snapshot},indent=2)+'\n')
if before:
 assert snapshot['old_event_hash']==before['snapshot']['old_event_hash'],'old payloads changed'
 assert snapshot['map_hash']==before['snapshot']['map_hash'],'existing map changed'
 prior={f['id']:f for f in before['snapshot']['founders']}
 for f in snapshot['founders']:
  if f['id'] in prior:
   old=prior[f['id']]
   assert f['silver_amount']==old['silver_amount'] and f['silver_rate']==old['silver_rate'],'founder silver changed'
 assert snapshot['migration']=={'version':179,'dirty':False},snapshot['migration']
 print('PASS all existing map rows + old event payloads + surviving founder silver preserved; migration179 clean')
else:print('PASS read-only predeploy health/snapshot')
