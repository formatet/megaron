#!/usr/bin/env python3
"""Read-only measurement of PRIVATE live dump on disposable PG16. No live DB writes.
Usage measure_snapshot.py PRIVATE.dump PUBLIC-output.json
"""
from pathlib import Path
import hashlib,json,secrets,subprocess,sys,time
source=Path(sys.argv[1]).resolve();out=Path(sys.argv[2]).resolve();name='sitos-measure-'+secrets.token_hex(5)
def run(args,**kw):return subprocess.run(args,check=True,capture_output=True,**kw)
def sql(q):return run(['docker','exec',name,'psql','-U','postgres','-d','snapshot','-Atc',q],text=True).stdout.strip()
try:
 run(['docker','run','-d','--rm','--name',name,'-e','POSTGRES_PASSWORD=private','-e','POSTGRES_DB=snapshot','postgres:16-alpine'])
 for _ in range(100):
  if subprocess.run(['docker','exec',name,'pg_isready','-h','127.0.0.1','-U','postgres','-q'],capture_output=True).returncode==0:break
  time.sleep(.2)
 with source.open('rb') as f:run(['docker','exec','-i',name,'pg_restore','-U','postgres','-d','snapshot','--no-owner','--exit-on-error'],stdin=f)
 cities=json.loads(sql("""SELECT json_agg(x) FROM(SELECT s.id,s.name,s.population,w.current_tick,
 (SELECT json_agg(json_build_object('good',g.good_key,'amount',g.amount,'rate',g.rate,'calc_tick',g.calc_tick,'settled',settled(g.amount,g.rate,g.calc_tick))) FROM settlement_goods g WHERE g.settlement_id=s.id AND g.good_key IN ('grain','fish')) goods,
 (SELECT json_object_agg(g.good_key,g.amount) FROM settlement_granary g WHERE g.settlement_id=s.id) granary,
 (SELECT json_agg(json_build_object('id',u.id,'type',u.type,'size',u.size,'crew',u.crew,'status',u.status,'settlement',u.settlement_id,'payer',u.support_settlement_id)) FROM units u WHERE u.home_settlement_id=s.id OR u.settlement_id=s.id OR u.support_settlement_id=s.id) units
 FROM settlements s JOIN worlds w ON w.id=s.world_id WHERE s.name IN ('Phaistos','Scheria','Tylissos'))x"""))
 daily=json.loads(sql("""SELECT json_agg(x ORDER BY name,world_tick) FROM(SELECT s.name,e.world_tick,json_agg(json_build_object('event',e.event_type,'stream',e.stream_id,'payload',e.payload) ORDER BY e.created_at,e.id) outcomes
 FROM settlements s JOIN events e ON e.stream_id=s.id OR e.stream_id IN(SELECT u.id FROM units u WHERE u.home_settlement_id=s.id OR u.settlement_id=s.id OR u.support_settlement_id=s.id)
 WHERE s.name IN ('Phaistos','Scheria') AND e.world_tick BETWEEN 300 AND 400 AND e.event_type IN ('StarvationDamage','UnitAttrition','UpkeepSettled','SitosGranaryStored','SitosGranaryReleased') GROUP BY s.name,e.world_tick)x"""))
 result={'dump_sha256':hashlib.sha256(source.read_bytes()).hexdigest(),'schema':sql('SELECT version,dirty FROM schema_migrations'),'settlements_missing_grain':int(sql("SELECT count(*) FROM settlements s WHERE s.state != 'sunk' AND NOT EXISTS(SELECT 1 FROM settlement_goods g WHERE g.settlement_id=s.id AND g.good_key='grain')")),'read_only':True,'cities':cities,'daily_events':daily,'limitations':'Historical events are outcomes, not a complete stored-rate history; unit association uses current home/payer/settlement. Do not infer 62 losses from 62 audit StarvationDamage entries.'}
 out.parent.mkdir(parents=True,exist_ok=True);out.write_text(json.dumps(result,indent=2))
 for c in cities:print(c['name'],'pop',c['population'],'tick',c['current_tick'],'granary',c['granary'])
 for city in ('Phaistos','Scheria'):
  counts={}
  for d in daily:
   if d['name']==city:
    for e in d['outcomes']:counts[e['event']]=counts.get(e['event'],0)+1
  print(city,counts)
finally:subprocess.run(['docker','rm','-fv',name],capture_output=True)
