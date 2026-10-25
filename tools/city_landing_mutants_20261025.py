#!/usr/bin/env python3
"""Physical city-landing mutations; every arm gets a fresh migrated database."""
from pathlib import Path
import subprocess,json,os,socket
root=Path(__file__).resolve().parents[1];out=root/'docs/reviews/landsatt-i-stad'
cases=[
 ('docking-city','server/internal/combat/march_start.go','if dockMission {','if false && dockMission {','./internal/combat/','TestCityLandingAllPassengers'),
 ('all-passengers','server/internal/combat/city_disembark.go','for _, id := range ids {','if len(ids)>1 { ids=ids[:1] }; for _, id := range ids {','./internal/combat/','TestCityLandingAllPassengers'),
 ('preserve-support','server/internal/combat/unit_arrival.go','settlement_id = $5,','settlement_id = $5, support_settlement_id = $5,','./internal/combat/','TestCityLandingAllPassengers'),
 ('owned-port','server/internal/combat/ship_port_call.go','s.owner_id=$2 AND s.state','s.state','./internal/combat/','TestCityLandingRechecksOwnership'),
 ('nearer-food-stop','server/internal/combat/return_food.go',' || *intent == "dock"','','./internal/combat/','TestCityLandingAllPassengers'),
 ('private-harbour','server/api/handlers/world.go','(s.owner_id=$2 AND EXISTS','(true AND EXISTS','./api/handlers/','TestCitiesOwnPortMetadata'),
]
results=[]
for name,file,old,new,pkg,test in cases:
 p=root/file;original=p.read_bytes();source=original.decode();assert source.count(old)==1,(name,source.count(old))
 try:
  p.write_text(source.replace(old,new))
  with socket.socket() as s:s.bind(('127.0.0.1',0));port=s.getsockname()[1]
  run=subprocess.run([str(root/'tools/gotest.sh'),pkg,'-run',test],cwd=root,capture_output=True,text=True,env={**os.environ,'GOTEST_PORT':str(port)})
  log=run.stdout+run.stderr;(out/(name+'-mutant.log')).write_text(log)
  assert run.returncode!=0 and '--- FAIL: TestCit' in log and '[build failed]' not in log and 'panic:' not in log,(name,log)
  results.append({'name':name,'exit':run.returncode,'assertion_red':True})
 finally:p.write_bytes(original);assert p.read_bytes()==original
(out/'mutations.json').write_text(json.dumps(results,indent=2)+'\n')
print('PASS six assertion-red city-landing mutations, exact restore')
