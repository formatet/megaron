#!/usr/bin/env python3
"""Physical source/migration mutations, named DB-test failures and byte restore."""
from pathlib import Path
import json, os, socket, subprocess, sys
ROOT=Path(__file__).resolve().parents[1]
OUT=ROOT/'docs/reviews/silverkalibrering/mutations';OUT.mkdir(parents=True,exist_ok=True)
cases=[
 ('raw_work_reference','server/db/migrations/177_labor_values_silver.up.sql','ELSE 1 END;','ELSE 2 END;',['./internal/religion'],'TestSilverCalibrationLaborIndex','TestSilverCalibrationLaborIndexNeverSmoothsHistoricalPrices'),
 ('legacy_price_reader','server/internal/religion/index.go','        g.labor_value,','        g.base_value,',['./internal/religion'],'TestSilverCalibrationLaborIndex','TestSilverCalibrationLaborIndexNeverSmoothsHistoricalPrices'),
 ('inland_endowment','server/internal/economy/genesis_silver.go','MetropolisSilverInland  = 730.0','MetropolisSilverInland  = 729.0',['./api/handlers'],'TestSilverCalibrationFirstFoundingEndowment','TestSilverCalibrationFirstFoundingEndowment'),
 ('coastal_endowment','server/internal/economy/genesis_silver.go','MetropolisSilverCoastal = 1278.0','MetropolisSilverCoastal = 1277.0',['./api/handlers'],'TestSilverCalibrationFirstFoundingEndowment','TestSilverCalibrationFirstFoundingEndowment'),
 ('founding_double_credit','server/api/handlers/found_metropolis.go','map[string]float64{"grain": grainLeft}','map[string]float64{"grain": grainLeft, "silver": silverLeft}',['./api/handlers'],'TestSilverCalibrationFirstFoundingEndowment','TestSilverCalibrationFirstFoundingEndowment'),
 ('refugee_no_extra_mint','server/internal/economy/genesis_silver.go','if !refugee {','if true {',['./api/handlers'],'TestSilverCalibrationRefugeeFoundingNeverMints','TestSilverCalibrationRefugeeFoundingNeverMints'),
 ('colony_cap','server/internal/economy/genesis_silver.go','math.Max(60, math.Max(2*purse, stock))','math.Max(30, math.Max(purse, stock))',['./internal/combat','./internal/economy'],'TestFoundColony_DoesNotMintSilver|TestSilverCalibrationColonyCap','TestFoundColony_DoesNotMintSilver'),
 ('bronze_work_chain','server/db/migrations/177_labor_values_silver.up.sql',"WHEN 'bronze' THEN 11","WHEN 'bronze' THEN 12",['./internal/religion'],'TestSilverCalibrationLaborIndex','TestSilverCalibrationLaborIndexNeverSmoothsHistoricalPrices'),
 ('stone_work_reference','server/db/migrations/177_labor_values_silver.up.sql',"WHEN 'stone' THEN 2","WHEN 'stone' THEN 1",['./internal/religion'],'TestSilverCalibrationLaborIndex','TestSilverCalibrationLaborIndexNeverSmoothsHistoricalPrices'),
 ('hills_miner_anchor','server/db/migrations/177_labor_values_silver.up.sql',"WHEN 'hills' THEN 1.0/1.7","WHEN 'hills' THEN 2.0/1.7",['./internal/economy'],'TestSilverCalibrationMineAnchors','TestSilverCalibrationMineAnchorsAndMaxLevel'),
 ('limestone_miner_anchor','server/db/migrations/177_labor_values_silver.up.sql',"WHEN 'mountain_limestone' THEN 2.5/1.7","WHEN 'mountain_limestone' THEN 5.0/1.7",['./internal/economy'],'TestSilverCalibrationMineAnchors','TestSilverCalibrationMineAnchorsAndMaxLevel'),
 ('silver_no_floor','server/internal/religion/offering.go','value, known := divineValues[good]','value, known := divineValues[good]\n if good == "silver" { value=math.Max(value,100) }',['./internal/religion','./api/handlers'],'TestSilverCalibration','TestSilverCalibrationOfferWorkAndLegacyPayloadRead'),
 ('kharis_shared_reference','server/internal/kharis/tick.go','baseValues, err := religion.LoadLaborValues(ctx, h.pool)','baseValues, err := religion.LoadLaborValues(ctx, h.pool)\n baseValues=map[string]float64{"oil":4,"wine":5}',['./internal/kharis'],'TestSilverCalibrationKharisFlow','TestSilverCalibrationKharisFlowLeavesStalePriceClamp'),
 ('v2_smoothing_float','server/internal/religion/index.go','(1 - $6::float8) + EXCLUDED.divine_value * $6::float8','(1 - $6) + EXCLUDED.divine_value * $6',['./internal/religion'],'TestSilverCalibrationLaborIndex','TestSilverCalibrationLaborIndexNeverSmoothsHistoricalPrices'),
]
def fresh(args):
 with socket.socket() as sock:sock.bind(('127.0.0.1',0));port=sock.getsockname()[1]
 return subprocess.run([str(ROOT/'tools/gotest.sh'),*args],cwd=ROOT,env={**os.environ,'GOTEST_PORT':str(port)},capture_output=True,text=True)
selected=set(sys.argv[1:])
if selected:
 assert selected <= {c[0] for c in cases}, selected
 cases=[c for c in cases if c[0] in selected]
results=[]
for name,path,needle,replacement,packages,pattern,expected in cases:
 p=ROOT/path;original=p.read_bytes();assert original.decode().count(needle)==1,(name,'nonunique source anchor')
 try:
  p.write_text(original.decode().replace(needle,replacement,1))
  r=fresh([*packages,'-run',pattern]);(OUT/(name+'-red.log')).write_text(r.stdout+r.stderr)
  assert r.returncode and '--- FAIL: '+expected in r.stdout,(name,r.stdout,r.stderr)
 finally:p.write_bytes(original)
 assert p.read_bytes()==original
 results.append({'mutation':name,'named_red':expected,'source_byte_restored':True})
 print(name,'NAMED RED; BYTE RESTORED',flush=True)
r=fresh(['./internal/economy','./internal/religion','./internal/kharis','./internal/combat','./api/handlers','-run','TestSilverCalibration|TestAlphaOfferDebits|TestFoundColony'])
(OUT/'restored-fresh-green.log').write_text(r.stdout+r.stderr);assert r.returncode==0,(r.stdout,r.stderr)
for case in results:case['restored_fresh_green']=True
result_file=OUT/'results.json'
if selected and result_file.exists():
 prior=json.loads(result_file.read_text());results=[r for r in prior if r['mutation'] not in selected]+results
result_file.write_text(json.dumps(results,indent=2)+'\n')
print('ALL RESTORED; FRESH GREEN',flush=True)
