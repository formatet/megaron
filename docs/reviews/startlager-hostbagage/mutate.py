#!/usr/bin/env python3
"""Physical old top-up and both grain seeds, named DB failures and exact restoration."""
import pathlib,os,subprocess
HERE=pathlib.Path(__file__).resolve().parent;ROOT=HERE.parents[2]
clean={k:os.environ[k] for k in ('HOME','PATH')}
cases=[
 ('silver-topup','server/internal/economy/genesis_silver.go','amount = math.Max(0, carried)','amount = math.Max(730, carried)',['./api/handlers'],'TestSilverCalibrationFirstFoundingCarriedOnly'),
 ('capital-grain8','server/api/handlers/create_metropolis.go',"WHEN 'grain'     THEN 0","WHEN 'grain'     THEN 8",['./api/handlers'],'TestSilverCalibrationFirstFoundingCarriedOnly'),
 ('colony-grain8','server/internal/economy/catchment_preview.go','ColonyGrainSeed = 0','ColonyGrainSeed = 8',['./internal/combat'],'TestFoundColony_DoesNotMintSilver'),
]
for name,path,needle,replacement,packages,test in cases:
 p=ROOT/path;original=p.read_bytes();assert original.decode().count(needle)==1
 cmd=[str(ROOT/'tools/gotest.sh'),*packages,'-run','^'+test+'$','-v']
 try:
  p.write_text(original.decode().replace(needle,replacement))
  r=subprocess.run(cmd,cwd=ROOT,env=clean,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True)
  (HERE/(name+'-red.log')).write_text(r.stdout)
  assert r.returncode and '--- FAIL: '+test in r.stdout and '[build failed]' not in r.stdout,r.stdout
 finally:p.write_bytes(original)
 assert p.read_bytes()==original
 r=subprocess.run(cmd,cwd=ROOT,env=clean,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True)
 (HERE/(name+'-restored-green.log')).write_text(r.stdout);assert r.returncode==0,r.stdout
 print(name+': named RED -> exact bytes -> fresh GREEN',flush=True)
