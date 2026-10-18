#!/usr/bin/env python3
"""Read-only production dump -> private disposable PG16; publish game risk only."""
from pathlib import Path
import subprocess,json,secrets,time,sys
root=Path(__file__).resolve().parent
name='kharis-risk-'+secrets.token_hex(5)
def run(a,**kw):return subprocess.run(a,check=True,capture_output=True,**kw)
try:
 run(['docker','run','-d','--rm','--name',name,'-e','POSTGRES_PASSWORD=private','-e','POSTGRES_DB=snapshot','postgres:16-alpine'])
 for _ in range(100):
  if subprocess.run(['docker','exec',name,'pg_isready','-U','postgres','-q'],capture_output=True).returncode==0:break
  time.sleep(.2)
 with Path(sys.argv[1]).open('rb') as f:run(['docker','exec','-i',name,'pg_restore','-U','postgres','-d','snapshot','--no-owner','--exit-on-error'],stdin=f)
 q="""SELECT COALESCE(json_agg(x ORDER BY world_id,wanax),'[]') FROM (
 SELECT pwr.world_id,p.username wanax,w.current_tick,pwr.status,
 GREATEST(0,pwr.kharis_amount+pwr.kharis_rate*GREATEST(0,w.current_tick-pwr.kharis_calc_tick)) kharis,
 (SELECT COUNT(*) FROM settlements s WHERE s.world_id=pwr.world_id AND s.owner_id=pwr.player_id AND s.state NOT IN('sunk','collapsed','razed')) cities,
 EXISTS(SELECT 1 FROM settlements s WHERE s.world_id=pwr.world_id AND s.owner_id=pwr.player_id AND s.is_capital) has_capital
 FROM player_world_records pwr JOIN players p ON p.id=pwr.player_id JOIN worlds w ON w.id=pwr.world_id WHERE w.status='active'
 )x"""
 rows=json.loads(run(['docker','exec',name,'psql','-U','postgres','-d','snapshot','-Atc',q],text=True).stdout)
 for r in rows:
  k=r['kharis'];n=r['cities'];r['band']='Wrathful' if k<5 else 'Suspicious' if k<30 else 'Indifferent' if k<60 else 'Favorable'
  r['punishment_probability_per_tick']=(.3 if k<5 else .1 if k<30 else 0) if r['has_capital'] else 0
  r['expected_roll_hits_next_16_if_band_unchanged']=16*r['punishment_probability_per_tick']
  r['decay_per_tick']=.1+.1*n*(n-1)/2
 out={'source':'fresh read-only pg_dump; private PG16 restoration; no live writes','schema':184,'scope':'all records in active worlds; actual current kharis, no lift to40','risk_interpretation':'Bernoulli roll hazard; actual damage requires eligible goods/garrison. Current band fixed for 16-tick expectation, not forecast of changing kharis. Hosts without capital are not maintained by current handler.','counts':{band:sum(r['has_capital'] and r['band']==band for r in rows) for band in ('Wrathful','Suspicious','Indifferent','Favorable')},'records':rows}
 (root/'live-risk.json').write_text(json.dumps(out,indent=2)+'\n')
 print(json.dumps(out,indent=2))
finally:
 subprocess.run(['docker','rm','-fv',name],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
