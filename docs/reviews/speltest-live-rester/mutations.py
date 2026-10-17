#!/usr/bin/env python3
from pathlib import Path
import hashlib,os,socket,subprocess
ROOT=Path(__file__).resolve().parents[3];OUT=Path(__file__).resolve().parent
DB=['tools/gotest.sh','./api/handlers','-run','TestFoundingTimberWarning|TestMarchPreview_ReadOnlyAndTiming|TestPlaytestKharis|TestPlaytestColonizePreview']
CLI=['go','-C','server','test','-count=1','./cmd/keryx','-run','TestLiveRemainder']
JS=['node','--test','web/static/js/megaron/ui/drawers/kult_values.test.mjs','web/static/js/megaron/ui/marchctx.test.mjs','web/static/js/megaron/ui/march_preview.test.mjs']
cases=[
 ('budget','server/api/handlers/unit_march_preview.go','finance["planned_expedition_ticks"] = order.ExpeditionTicks','finance["planned_expedition_ticks"] = 0',DB,'TestMarchPreview_ReadOnlyAndTiming'),
 ('home','server/api/handlers/unit_march_preview.go','currentTick + order.ExpeditionTicks','currentTick',DB,'TestMarchPreview_ReadOnlyAndTiming'),
 ('no-hidden-arrival','server/api/handlers/unit_march_preview.go','finance["planned_expedition_ticks"] = order.ExpeditionTicks','finance["planned_expedition_ticks"] = order.ExpeditionTicks; finance["arrival_tick"] = 123',DB,'TestMarchPreview_ReadOnlyAndTiming'),
 ('cedar','server/api/handlers/world.go',' || goods["cedar"] > 0','',DB,'TestFoundingTimberWarning_WoodFogAndCedar'),
 ('fog','server/api/handlers/world.go','if unknown > 0 {','if false {',DB,'TestFoundingTimberWarning_WoodFogAndCedar'),
 ('timber-wire','server/api/handlers/world.go','foundingTimberWarning(goods, unknown),','"",',DB,'TestPlaytestColonizePreview_FullConsumption'),
 ('values-wire','server/api/handlers/province.go','"divine_values":                    divineValues,','"divine_values":                    map[string]float64{},',DB,'TestPlaytestKharis_SameRealmPoolInTwoCities'),
 ('goods-word','server/cmd/keryx/cmd_goods.go','"Marg/g")','"Workdays/unit")',CLI,'TestLiveRemainder_GoodsDoesNotPresentTechnicalValuation'),
 ('cli-values','server/cmd/keryx/cmd_rite.go','key, values[key])','key, 1.0)',CLI,'TestLiveRemainder_DivineValuesAtAltar'),
 ('cli-home','server/cmd/keryx/cmd_march_preview.go','if forecast.PlannedHomeByTick != nil {','if false {',CLI,'TestLiveRemainder_ConditionalExpeditionAndTimber'),
 ('web-values','web/static/js/megaron/ui/drawers/kult_values.js',"value.toLocaleString('en-US', {maximumSignificantDigits:4})",'0',JS,'altar shows current divine values'),
 ('host-warning','web/static/js/megaron/render/map.js',"p?.timber_warning ?",'false ?',JS,'Host summary exposes the wood warning'),
]
for name,relative,old,new,command,assertion in cases:
 p=ROOT/relative;original=p.read_bytes();source=original.decode();assert source.count(old)==1,(name,source.count(old))
 def run(suffix,green):
  env={k:os.environ[k] for k in ('HOME','PATH')}
  if command[0]=='tools/gotest.sh':
   with socket.socket() as sock:sock.bind(('127.0.0.1',0));env['GOTEST_PORT']=str(sock.getsockname()[1])
  result=subprocess.run(command,cwd=ROOT,env=env,capture_output=True,text=True);log=result.stdout+result.stderr
  (OUT/(name+'-'+suffix+'.log')).write_text(log)
  if green:assert result.returncode==0,(name,log[-2000:])
  else:assert result.returncode and assertion in log and ('--- FAIL:' in log or 'not ok' in log),(name,log[-2000:])
 try:p.write_text(source.replace(old,new));run('red',False)
 finally:p.write_bytes(original)
 run('restored',True);assert p.read_bytes()==original
 print(name,'named RED -> identical bytes -> GREEN',hashlib.sha256(original).hexdigest(),flush=True)
