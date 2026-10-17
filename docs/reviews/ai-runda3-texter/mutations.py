#!/usr/bin/env python3
"""Physical, compiling mutations; named assertion RED -> exact bytes -> GREEN."""
from pathlib import Path
import hashlib,os,socket,subprocess,sys
ROOT=Path(__file__).resolve().parents[3];OUT=Path(__file__).resolve().parent
cli=lambda name:['go','-C','server','test','./cmd/keryx','-run',name,'-count=1','-v']
api=['tools/gotest.sh','./api/handlers','-run','TestAIRound3_FoodAfterArmyAPIParity','-v']
js=['node','--test','web/static/js/megaron/ui/drawers/city_production.test.mjs','web/static/js/megaron/ui/drawers/economy.test.mjs']
cases=[
 ('api-army-drain','server/api/handlers/province.go','return foodNet - grainUpkeep','return foodNet',api,'TestAIRound3_FoodAfterArmyAPIParity'),
 ('cli-army-net','server/cmd/keryx/cmd_status.go','sett["food_net_per_tick_after_upkeep"].(float64)','sett["net_grain_per_tick_after_upkeep"].(float64)',cli('TestAIRound3_StatusArmyAndSingleFoodAdvice'),'TestAIRound3_StatusArmyAndSingleFoodAdvice'),
 ('cli-food-advice','server/cmd/keryx/cmd_status.go','if isFoodGood(k) {','if false {',cli('TestAIRound3_StatusArmyAndSingleFoodAdvice'),'TestAIRound3_StatusArmyAndSingleFoodAdvice'),
 ('city-flag','server/cmd/keryx/cmd_city.go','cmd.Flags().StringVar(&provinceID, "province", "", "city name or province ID (default: capital)")','// omitted flag',cli('TestAIRound3_ProvinceForms'),'TestAIRound3_ProvinceForms'),
 ('status-positional','server/cmd/keryx/cmd_status.go','if len(args) > 0 {','if false {',cli('TestAIRound3_ProvinceForms'),'TestAIRound3_ProvinceForms'),
 ('build-positional','server/cmd/keryx/cmd_build.go','if len(args) > 0 {','if false {',cli('TestAIRound3_ProvinceForms'),'TestAIRound3_ProvinceForms'),
 ('place-positional','server/cmd/keryx/cmd_place.go','if len(args) >= 3 {','if false {',cli('TestAIRound3_ProvinceForms'),'TestAIRound3_ProvinceForms'),
 ('goods-scale','server/cmd/keryx/cmd_goods.go','workersStr := fmt.Sprintf("%d", int(employed)/100)','workersStr := fmt.Sprintf("%d", int(employed))',cli('TestAIRound3_GoodsCitizenScale'),'TestAIRound3_GoodsCitizenScale'),
 ('empty-remove-hint','server/cmd/keryx/cmd_place.go','fmt.Println(occupiedGoodHint(opts, roll))','fmt.Println("No hint")',cli('TestAIRound3_EmptyUnplacePointsToOccupiedGood'),'TestAIRound3_EmptyUnplacePointsToOccupiedGood'),
 ('foreign-remove-hint','server/cmd/keryx/cmd_gubbe.go','if h.HeldBy != "" {','if false {',cli('TestAIRound3_ForeignWorkersAreNotRemovalTargets'),'TestAIRound3_ForeignWorkersAreNotRemovalTargets'),
 ('eta-clock','server/cmd/keryx/output.go','return fmt.Sprintf("in %s", formatDays(whole, "%d"))','return fmt.Sprintf("in %s (%s)", formatDays(whole, "%d"), wallClock(t))',cli('TestAIRound3_ETAOnlyGameDays'),'TestAIRound3_ETAOnlyGameDays'),
 ('wait-copy','server/cmd/keryx/client.go','if nothing else uses it','before other use',cli('TestAIRound3_ETAOnlyGameDays'),'TestAIRound3_ETAOnlyGameDays'),
 ('web-city-army','web/static/js/megaron/ui/drawers/city_production.js','pd.food_net_per_tick_after_upkeep ?? s.food_net_per_tick','s.food_net_per_tick',js,'AIRound3'),
 ('web-overview-army','web/static/js/megaron/ui/drawers/economy.js','pd.food_net_per_tick_after_upkeep ?? sitos.food_net_per_tick ?? netTick','sitos.food_net_per_tick ?? netTick',js,'AIRound3'),
]
cases += [
 ('yes-no-question','server/cmd/keryx/cmd_founding.go','if !yes {\n\t\t\t\t\tif !stdinIsTerminal()', 'if true {\n\t\t\t\t\tif !stdinIsTerminal()',cli('TestAIRound3_YesDoesNotAsk'),'TestAIRound3_YesDoesNotAsk'),
 ('foreign-stars','server/cmd/keryx/cmd_city.go','if !held && surplus > 0','if surplus > 0',cli('TestAIRound3_ForeignWorkersAreNotRemovalTargets'),'TestAIRound3_ForeignWorkersAreNotRemovalTargets'),
]
cases += [('army-priority','server/cmd/keryx/cmd_status.go','armyUpkeepWarning(grainProduced-armyGrain, netS, grainStock, silverStock)','armyUpkeepWarning(grainProduced-armyGrain-5, netS, grainStock, silverStock)',cli('TestAIRound3_ArmyGrainIsPaidBeforePeople'),'TestAIRound3_ArmyGrainIsPaidBeforePeople')]
cases=cases[int(sys.argv[1]):] if len(sys.argv)>1 else cases
results=[]
for name,relative,old,new,cmd,named in cases:
 p=ROOT/relative;original=p.read_bytes();source=original.decode();assert source.count(old)==1,(name,source.count(old))
 def run(label,green):
  env={k:os.environ[k] for k in ('HOME','PATH')}
  if cmd[0]=='tools/gotest.sh':
   with socket.socket() as sock:sock.bind(('127.0.0.1',0));env['GOTEST_PORT']=str(sock.getsockname()[1])
  r=subprocess.run(cmd,cwd=ROOT,env=env,capture_output=True,text=True);log=r.stdout+r.stderr;(OUT/(name+'-'+label+'.log')).write_text(log)
  if green:assert r.returncode==0,(name,log[-3000:])
  else:assert r.returncode and named in log and ('--- FAIL:' in log or 'not ok' in log) and '[build failed]' not in log,(name,log[-3000:])
 try:p.write_text(source.replace(old,new));run('red',False)
 finally:p.write_bytes(original)
 run('restored',True);assert p.read_bytes()==original
 print(name,'named assertion RED -> exact bytes -> GREEN',hashlib.sha256(original).hexdigest(),flush=True)
