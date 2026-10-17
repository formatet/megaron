#!/usr/bin/env python3
from pathlib import Path
import hashlib,os,socket,subprocess,sys
ROOT=Path(__file__).resolve().parents[3];OUT=Path(__file__).resolve().parent
CLI=['go','-C','server','test','-count=1','./cmd/keryx','-run','TestAIRound|TestMarchPreviewNeverDispatches']
DB=['tools/gotest.sh','./api/handlers','-run','TestAIRound|TestMarchPreview_ReadOnlyAndTiming|TestOpeningDispatch|TestSettlementsOverviewParity']
JS=['node','--test','web/static/js/megaron/ui/marchctx.test.mjs','web/static/js/megaron/ui/format.test.mjs','web/static/js/megaron/ui/drawers/economy.test.mjs']
cases=[
 ('dust-runway','server/internal/economy/founding_forecast.go','net >= -ForecastBalanceEpsilon','net >= 0',['go','-C','server','test','./internal/economy','-run','TestFoodRunwayTicks_NoDustInfinity','-count=1'],'TestFoodRunwayTicks_NoDustInfinity'),
 ('farm-fact','server/api/handlers/world.go','starterFarm && !coastalCentre && farmGift,','false,',DB,'TestAIRound_ForecastReportsFarmFacts'),
 ('farm-cli','server/cmd/keryx/cmd_unit.go','} else if p.Grain.StarterFarmIncluded {','} else if false {',CLI,'TestAIRound_FarmAlreadyIncluded'),
 ('food-status','server/cmd/keryx/cmd_status.go','netTick = full','netTick = full - 999',CLI,'TestAIRound_StatusUsesFoodAndShowsGranary'),
 ('active-dispatch','server/api/handlers/opening_dispatch.go','state == "forming")','true)',DB,'TestOpeningDispatchJoinConcurrentRetryRiseAndPrivacy'),
 ('world-select','server/cmd/keryx/main.go','cfg.WorldID = id','cfg.WorldID = ""',CLI,'TestAIRound_WorldSelectionAndPreserveConfig'),
 ('city-coordinate','server/cmd/keryx/cmd_city.go','h.HexOrdinal, h.HexQ, h.HexR,','h.HexOrdinal, 0, 0,',CLI,'TestAIRound_CityCoordinatesAndExploreArea'),
 ('budget-preview','server/api/handlers/unit_march_preview.go','if _, err := combat.PreviewMarch(ctx, h.pool, h.clk, order, targetKnown); err != nil {','if _, err := combat.PreviewMarch(ctx, h.pool, h.clk, order, targetKnown); false {',DB,'TestMarchPreview_ReadOnlyAndTiming'),
 ('no-confirm','server/cmd/keryx/cmd_unit.go','if !yes && !stdinIsTerminal() {','if false {',CLI,'TestAIRound_ColonizeConfirmationAndHost'),
 ('rate-wire','server/api/handlers/helpers.go','Need: qty, Have: have, Rate: rate','Need: qty, Have: have, Rate: 0',DB,'TestAIRound_ShortfallCarriesCurrentRate'),
 ('rate-cli','server/cmd/keryx/client.go','if m.Rate > 0 {','if false {',CLI,'TestAIRound_ShortfallProductionWait'),
 ('sitos-wire','server/api/handlers/settlement.go','GranaryPerGood: fs.GranaryPerGood,','GranaryPerGood: nil,',DB,'TestSettlementsOverviewParity_MatchesProvinceGet'),
 ('host-colonize','server/cmd/keryx/cmd_unit.go','u.ID == unitID && u.Type == "nomadic_host"','u.ID == unitID && false',CLI,'TestAIRound_ColonizeConfirmationAndHost'),
 ('explore-area','server/cmd/keryx/cmd_unit.go','u.Expedition.AreaQ, u.Expedition.AreaR','0, 0',CLI,'TestAIRound_CityCoordinatesAndExploreArea'),
 ('web-farm','web/static/js/megaron/ui/marchctx.js','g.starter_farm_included ?','false ?',JS,'AI forecast avoids dust horizons'),
 ('web-wait','web/static/js/megaron/ui/format.js','m.rate_per_tick > 0 ?','false ?',JS,'AI shortage gives time'),
 ('growth-zero','web/static/js/megaron/ui/drawers/city_production.js',"net !== 0 && Math.abs(net)<0.05 ? 'about even'","false ? 'about even'",['node','--test','web/static/js/megaron/ui/drawers/city_production.test.mjs'],'growth food balance does not print'),
 ('food-advice','server/cmd/keryx/cmd_status.go','ok && foodNet <= 0','ok && foodNet < -999',CLI,'TestAIRound_StatusUsesFoodAndShowsGranary'),
 ('web-food-column','web/static/js/megaron/ui/drawers/economy.js','grainRate: net,','grainRate: netTick,',JS,'settlement food column follows full fish-inclusive'),
]
selected=set(sys.argv[1:])
for name,relative,old,new,cmd,named in cases:
 if selected and name not in selected:continue
 p=ROOT/relative;original=p.read_bytes();source=original.decode();count=source.count(old);assert count>=1,(name,count)
 def run(suffix,green):
  env={k:os.environ[k] for k in ('HOME','PATH')}
  if cmd[0]=='tools/gotest.sh':
   with socket.socket() as s:s.bind(('127.0.0.1',0));env['GOTEST_PORT']=str(s.getsockname()[1])
  r=subprocess.run(cmd,cwd=ROOT,env=env,capture_output=True,text=True);log=r.stdout+r.stderr;(OUT/(name+'-'+suffix+'.log')).write_text(log)
  if green:assert r.returncode==0,(name,log[-2200:])
  else:assert r.returncode and named in log and ('--- FAIL:' in log or 'not ok' in log),(name,log[-2200:])
 try:p.write_text(source.replace(old,new));run('red',False)
 finally:p.write_bytes(original)
 run('restored',True);assert p.read_bytes()==original
 print(name,'named RED -> exact bytes -> fresh GREEN',hashlib.sha256(original).hexdigest(),flush=True)
