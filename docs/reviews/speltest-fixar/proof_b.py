#!/usr/bin/env python3
"""Physical mutations of forecast, warnings, catalogue, threshold, notice and CLI errors."""
from pathlib import Path
import hashlib, os, subprocess, socket
ROOT=Path(__file__).resolve().parents[3];OUT=Path(__file__).resolve().parent
CLI=['go','-C','server','test','-count=1','./cmd/keryx','-run','TestPlaytest']
DB=['tools/gotest.sh','./api/handlers','-run','TestPlaytest']
JS=['node','--test','web/static/js/megaron/ui/marchctx.test.mjs']
cases=[
 ('centre','server/api/handlers/world.go', [('append([]hexgrid.Coord{center}, hexgrid.Ring(center, hexgrid.CatchmentRadius)...)','hexgrid.Disk(center, hexgrid.CatchmentRadius)')],DB,'TestPlaytestColonizePreview_CentreFirstAndCoastalGift'),
 ('consumption','server/api/handlers/world.go', [('foodForecast.Consumption,','0,')],DB,'TestPlaytestColonizePreview_FullConsumption'),
 ('reason','server/api/handlers/province.go',[('UnavailableReason: reason,','UnavailableReason: reason[:0],')],DB,'TestPlaytestKharis_SameRealmPoolInTwoCities'),
 ('catalogue','server/cmd/keryx/cmd_status.go',[('province.BuildingSpecs[kind].Costs["timber"]','0.0')],CLI,'TestPlaytestTimberWarning_FollowsCatalogue'),
 ('threshold','server/cmd/keryx/cmd_rite.go',[('math.Floor(value*10)/10','math.Round(value*10)/10')],CLI,'TestPlaytestKharisReading_DoesNotRoundUp'),
 ('uuid','server/cmd/keryx/notification_text.go',[('PoseidonGift json.RawMessage','PoseidonGift bool'),('len(b.PoseidonGift) > 0 && string(b.PoseidonGift) != "false" && string(b.PoseidonGift) != "null"','b.PoseidonGift')],CLI,'TestPlaytestNotifications_UUIDGiftAndInternalDelivery'),
 ('usage','server/cmd/keryx/main.go',[('SilenceUsage:  true,','SilenceUsage:  false,')],CLI,'TestPlaytestReadinessAndHelp'),
 ('warning','server/internal/loyalty/welfare.go',[('if grainWelfareStarving(stock, production, population) {','if false {')],['go','-C','server','test','-count=1','./internal/loyalty','-run','TestPlaytest'],'TestPlaytestFoundingWarning_MatchesGrainWelfare'),
 ('web-forecast','web/static/js/megaron/ui/marchctx.js',[('g.consumption_per_tick ?? (prodTick - netTick)','(prodTick - netTick)')],JS,'playtest: coastal forecast'),
 ('cli-forecast','server/cmd/keryx/cmd_unit.go',[('consPerTick = *p.Grain.ConsumptionPerTick','consPerTick = 0')],CLI,'TestPlaytestForecast_FullFoodDemandAndReserve'),
]
env={k:os.environ[k] for k in ('HOME','PATH')}
def run(command,name,green,assertion=''):
 runenv=env.copy()
 if command[0]=='tools/gotest.sh':
  with socket.socket() as sock:
   sock.bind(('127.0.0.1',0));runenv['GOTEST_PORT']=str(sock.getsockname()[1])
 result=subprocess.run(command,cwd=ROOT,env=runenv,capture_output=True,text=True)
 log=result.stdout+result.stderr;(OUT/(name+'.log')).write_text(log)
 if green:assert result.returncode==0,(name,log[-1500:])
 else:assert result.returncode!=0 and assertion in log and ('--- FAIL:' in log or 'not ok' in log),(name,log[-1500:])
for name,path,replacements,command,assertion in cases:
 p=ROOT/path;original=p.read_bytes();mutant=original.decode()
 for old,new in replacements:
  assert mutant.count(old)==1,(name,old,mutant.count(old));mutant=mutant.replace(old,new)
 try:p.write_text(mutant);run(command,'b-mutant-'+name,False,assertion)
 finally:p.write_bytes(original)
 run(command,'b-restored-'+name,True)
 assert p.read_bytes()==original
 print(name,'named RED -> identical bytes -> GREEN',hashlib.sha256(original).hexdigest(),flush=True)
