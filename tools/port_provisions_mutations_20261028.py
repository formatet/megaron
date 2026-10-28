#!/usr/bin/env python3
"""Physical port-provision mutants; isolated CHECKOUT and proof OUTPUT required."""
import pathlib,sys,subprocess,socket,hashlib,json
root,out=map(pathlib.Path,sys.argv[1:3]);out.mkdir(parents=True,exist_ok=True)
march='server/internal/combat/march_start.go';port='server/internal/economy/ship_port.go'
city='TestCityLandingAllPassengersStayAndSupportUnchanged';physical='TestShipDeparturePortRequiresPhysicalOwnedActiveCity'
cases=[('preview-remote',march,'economy.LoadFood(ctx, pool, *port, false)','economy.LoadFood(ctx, pool, *u.SupportSettlementID, false)',city),('dispatch-remote',march,'economy.TakeFood(ctx, tx, *port, provisions, false, true)','economy.TakeFood(ctx, tx, *u.SupportSettlementID, provisions, false, true)',city),('port-foreign-owner',port,'s.world_id=u.world_id AND s.owner_id=u.owner_id','s.world_id=u.world_id',physical),('port-position',port,' AND (u.q IS NULL OR u.q=p.map_q) AND (u.r IS NULL OR u.r=p.map_r)','',physical),('nil-support-skip',march,'unit.Status(currentStatus) == unit.StatusGarrison {','unit.Status(currentStatus) == unit.StatusGarrison && u.SupportSettlementID != nil {',city)]
def run(name,test):
 with socket.socket() as s:s.bind(('127.0.0.1',0));p=s.getsockname()[1]
 import os
 env=os.environ.copy();env['GOTEST_PORT']=str(p)
 result=subprocess.run(['tools/gotest.sh','./internal/combat','-run','^'+test+'$'],cwd=root,env=env,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
 (out/(name+'.log')).write_text(result.stdout);return result
summary=json.loads((out/'summary.json').read_text()) if (out/'summary.json').exists() else []
for name,file,old,new,test in cases:
 path=root/file;original=path.read_bytes();source=original.decode()
 if any(x['mutant']==name and x['exact_restored_sha256']==hashlib.sha256(original).hexdigest() for x in summary):continue
 assert source.count(old)==1,(name,source.count(old))
 try:
  path.write_text(source.replace(old,new,1));red=run(name+'-red',test)
 finally:path.write_bytes(original)
 assert path.read_bytes()==original
 assert red.returncode!=0 and ('--- FAIL: '+test) in red.stdout and 'build failed' not in red.stdout and 'panic:' not in red.stdout,(name,red.stdout[-1800:])
 green=run(name+'-restored-green',test);assert green.returncode==0,(name,green.stdout[-1800:])
 summary.append({'mutant':name,'file':file,'assertion':test,'red':True,'exact_restored_sha256':hashlib.sha256(original).hexdigest(),'fresh_restored_green':True});print(name+' RED / exact restore / GREEN',flush=True)
 (out/'summary.json').write_text(json.dumps(summary,indent=2)+'\n')
