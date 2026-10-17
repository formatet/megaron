#!/usr/bin/env python3
from pathlib import Path
import hashlib,os,socket,subprocess
ROOT=Path(__file__).resolve().parents[3];OUT=Path(__file__).resolve().parent
CLI=['go','-C','server','test','./cmd/keryx','-run','TestFoundingNotificationRunway_ClientUsesFoodNet','-count=1']
JS=['node','--test','web/static/js/megaron/ui/format.test.mjs']
raw="func() *float64 { if foodNet >= 0 {return nil}; n:=grainAmount / -foodNet; return &n }()"
cases=[]
for label,path,pkg in [('metropolis','server/api/handlers/found_metropolis.go','./api/handlers'),('colony','server/internal/combat/unit_arrival.go','./internal/combat')]:
 cmd=['tools/gotest.sh',pkg,'-run','TestFoundingNotificationRunway']
 cases += [(label+'-dust',path,'economy.FoodRunwayTicks(grainAmount, foodNet)',raw,cmd,'TestFoundingNotificationRunway'),(label+'-grain-only',path,'economy.FoodRunwayTicks(grainAmount, foodNet)','economy.FoodRunwayTicks(grainAmount, grainNet)',cmd,'TestFoundingNotificationRunway')]
cases += [
 ('notice-cli-dust','server/cmd/keryx/cmd_notifications.go','if math.Abs(net) < 1e-9 {','if false {',CLI,'TestFoundingNotificationRunway_ClientUsesFoodNet'),
 ('notice-cli-legacy-dust','server/cmd/keryx/cmd_notifications.go','if math.Abs(perTick) < 1e-9 {','if false {',CLI,'TestFoundingNotificationRunway_ClientUsesFoodNet'),
 ('notice-web-dust','web/static/js/megaron/ui/format.js','Math.abs(body.food_net_per_tick)<1e-9','false',JS,'founding notifications use food net'),
 ('notice-web-legacy-dust','web/static/js/megaron/ui/format.js','Math.abs(body.grain_net_per_tick)<1e-9','false',JS,'founding notifications use food net'),
]
for name,relative,old,new,cmd,named in cases:
 p=ROOT/relative;original=p.read_bytes();source=original.decode();assert source.count(old)==1,(name,source.count(old))
 def run(suffix,green):
  env={k:os.environ[k] for k in ('HOME','PATH')}
  if cmd[0]=='tools/gotest.sh':
   with socket.socket() as s:s.bind(('127.0.0.1',0));env['GOTEST_PORT']=str(s.getsockname()[1])
  r=subprocess.run(cmd,cwd=ROOT,env=env,capture_output=True,text=True);log=r.stdout+r.stderr;(OUT/(name+'-'+suffix+'.log')).write_text(log)
  if green:assert r.returncode==0,(name,log[-2000:])
  else:assert r.returncode and named in log and ('--- FAIL:' in log or 'not ok' in log),(name,log[-2000:])
 try:p.write_text(source.replace(old,new));run('red',False)
 finally:p.write_bytes(original)
 run('restored',True);assert p.read_bytes()==original
 print(name,'named RED -> exact bytes -> fresh GREEN',hashlib.sha256(original).hexdigest(),flush=True)
