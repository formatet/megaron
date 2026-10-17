#!/usr/bin/env python3
"""Physical mutants; fresh DB per arm, exact byte restore even after failure."""
from pathlib import Path
import hashlib,os,socket,subprocess
ROOT=Path(__file__).resolve().parents[3];OUT=Path(__file__).resolve().parent
DB=['tools/gotest.sh','./internal/loyalty','./api/handlers','-run','TestWelfareFoodTick|TestFoundingFoodWelfareWarning|TestColonizePreview_FishFoodWarning']
JS=['node','--test','web/static/js/megaron/ui/format.test.mjs']
CLI=['go','-C','server','test','-count=1','./cmd/keryx','-run','TestColonyFounded_FishFoodBalance']
cases=[
 ('fish-out','server/internal/economy/food_tick.go','FoodConsumptionSplit(demand, grainStock, fishStock, livestockStock)','FoodConsumptionSplit(demand, grainStock, 0, livestockStock)',DB,'TestWelfareFoodTick_FishAndReserves'),
 ('ignore-unmet','server/internal/loyalty/welfare.go','starving := w.foodUnmet > 0','starving := false',DB,'TestWelfareFoodTick_FishAndReserves'),
 ('preview-fish-out','server/api/handlers/world.go','foodForecast.GrainProduction+foodForecast.FishProduction, forecastPop','foodForecast.GrainProduction, forecastPop',DB,'TestColonizePreview_FishFoodWarning'),
 ('warning-off','server/internal/loyalty/welfare.go','if economy.FoodNet(foodRate, population) < 0 {','if false {',DB,'TestFoundingFoodWelfareWarning_MatchesFoodNet'),
 ('web-food-off','web/static/js/megaron/ui/format.js','if (body.food_net_per_tick != null) {','if (false) {',JS,'fish-fed colony uses additive full food balance'),
 ('cli-food-off','server/cmd/keryx/cmd_notifications.go','if body.FoodNetPerTick != nil {','if false {',CLI,'TestColonyFounded_FishFoodBalance'),
]
import sys
selected=set(sys.argv[1:])
for name,relative,old,new,cmd,named in cases:
 if selected and name not in selected:continue
 p=ROOT/relative;original=p.read_bytes();source=original.decode();assert source.count(old)==1,(name,source.count(old))
 def run(suffix,green):
  env={k:os.environ[k] for k in ('HOME','PATH')}
  if cmd[0]=='tools/gotest.sh':
   with socket.socket() as s:s.bind(('127.0.0.1',0));env['GOTEST_PORT']=str(s.getsockname()[1])
  r=subprocess.run(cmd,cwd=ROOT,env=env,capture_output=True,text=True);log=r.stdout+r.stderr;(OUT/(name+'-'+suffix+'.log')).write_text(log)
  if green:assert r.returncode==0,(name,log[-1800:])
  else:assert r.returncode and named in log and ('--- FAIL:' in log or 'not ok' in log),(name,log[-1800:])
 try:p.write_text(source.replace(old,new));run('red',False)
 finally:p.write_bytes(original)
 run('restored',True);assert p.read_bytes()==original
 print(name,'named RED -> identical bytes -> fresh GREEN',hashlib.sha256(original).hexdigest(),flush=True)
