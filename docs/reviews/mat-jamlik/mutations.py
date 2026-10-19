#!/usr/bin/env python3
"""Named domain mutations, restored byte-for-byte before a fresh database run."""
from pathlib import Path
import hashlib,json,os,re,socket,subprocess,sys
HERE=Path(__file__).resolve().parent;ROOT=HERE.parents[2]
cases=[
 ('grain-first','server/internal/economy/food_basket.go','if total > 0 && take > 0 {','if total > 0 && take > 0 {\n        remaining:=take', './internal/economy/','TestFoodConsumptionProportionalGrainAndFish'),
 ('stock-variety','server/internal/kharis/tick.go','FROM jsonb_each_text(s.food_consumed) AS meal(good,qty)','FROM (SELECT good_key,amount::text FROM settlement_goods WHERE settlement_id=s.id) AS meal(good,qty)', './internal/kharis/','TestGrowthVarietyReadsActualMealNotLaterStock'),
 ('crumbs-count-as-variety','server/internal/kharis/tick.go',"""AND meal.qty::numeric >= $8::numeric * (
		                   SELECT SUM(part.qty::numeric) FROM jsonb_each_text(s.food_consumed) AS part(good,qty)
		                   WHERE (part.good=ANY($4) OR part.good='livestock') AND part.qty::numeric>0
		               )""",'AND $8::numeric>0','./internal/kharis/','TestGrowthVarietyRequiresSubstantialMealShare'),
 ('fish-returned-as-grain','server/internal/combat/food_provisions.go','sid, good, n)','sid, economy.GoodGrain, n)', './internal/combat/','TestFishOnlyVoyagePreviewDispatchEatAndReturnSameGood'),
 ('special-home-unload-removed','server/internal/combat/unit_arrival.go','if port != nil && u.provisions > 0 {','if false {','./internal/combat/','TestSpecialHomeReturnUnloadsSameGoodsExactlyOnce'),
 ('cli-omits-wine','server/cmd/keryx/food_format.go','"oil", "wine", "livestock"','"oil", "livestock"','./cmd/keryx/','TestEqualFoodCityUsesServerTotalsAndGoods'),
 ('web-omits-wine','web/static/js/megaron/ui/food.js',"'oil', 'wine', 'livestock'","'oil', 'livestock'",None,'equal food: four stocks and actual meal use server totals, never grain-only estimates'),
]
selected=sys.argv[1:]
results=json.loads((HERE/'mutations.json').read_text()) if selected and (HERE/'mutations.json').exists() else []
for name,path,before,after,package,test in cases:
 if selected and name not in selected:continue
 results=[r for r in results if r['mutation']!=name]
 p=ROOT/path;original=p.read_bytes();assert original.count(before.encode())==1,(name,'ambiguous mutation')
 def run(phase):
  if package:
   with socket.socket() as listener:listener.bind(('127.0.0.1',0));port=listener.getsockname()[1]
   command=[str(ROOT/'tools/gotest.sh'),package,'-run','^'+test+'$'];env=dict(os.environ,GOTEST_PORT=str(port))
  else:
   command=['node','--experimental-default-type=module','--test','web/static/js/megaron/ui/drawers/city_production.test.mjs'];env={k:os.environ[k] for k in ['HOME','PATH']}
  result=subprocess.run(command,cwd=ROOT,env=env,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True)
  (HERE/(name+'-'+phase+'.log')).write_text(result.stdout)
  if package:assert 'färsk DB på migration 190' in result.stdout,(name,'no fresh190')
  return result
 try:
  changed=original.replace(before.encode(),after.encode())
  if name=='grain-first':
   changed=changed.replace(b'if n := math.Max(0, available[good]) * take / total;',b'if n := math.Min(math.Max(0, available[good]),remaining);').replace(b'out.Used[good] = n',b'out.Used[good] = n;remaining-=n',1)
  p.write_bytes(changed)
  red=run('red');named=('--- FAIL: '+test) in red.stdout if package else bool(re.search(r'not ok \d+ - '+re.escape(test),red.stdout));assert red.returncode!=0 and named,(name,'not named RED',red.stdout)
 finally:p.write_bytes(original)
 assert p.read_bytes()==original
 green=run('restored-green');assert green.returncode==0,(name,'restore failure',green.stdout)
 results.append(dict(mutation=name,test=test,source=path,sha256=hashlib.sha256(original).hexdigest(),result='named RED -> byte-exact restore -> fresh GREEN'))
 print(results[-1],flush=True)
(HERE/'mutations.json').write_text(json.dumps(results,indent=2)+'\n')
