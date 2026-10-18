#!/usr/bin/env python3
"""Physical compiling mutations; named assertion RED, exact restoration, fresh GREEN."""
from pathlib import Path
import hashlib,os,socket,subprocess,sys
ROOT=Path(__file__).resolve().parents[3];OUT=Path(__file__).resolve().parent
api=lambda name:['tools/gotest.sh','./api/handlers','-run',name,'-v']
econ=lambda name:['tools/gotest.sh','./internal/economy','-run',name,'-v']
cases=[('old-sum', 'server/internal/economy/automatic_food.go', 'best := 0.0', 'potential := 0.0\n\tfor _, opt := range options { potential += opt.RatePerGood[GoodGrain]*opt.MultPerGood[GoodGrain] }\n\ta.RemainderGrain = float64(a.RemainderPeople)/100*potential\n\treturn a\n\tbest := 0.0', ['tools/gotest.sh', './internal/economy', '-run', 'TestRestfolk_BestWorkerNotCatchmentSum', '-v'], 'TestRestfolk_BestWorkerNotCatchmentSum'), ('no-fish', 'server/internal/economy/automatic_food.go', '[]string{GoodGrain, GoodFish}', '[]string{GoodGrain}', ['tools/gotest.sh', './internal/economy', '-run', 'TestRestfolk_EligibleYieldAndBuildingBonus', '-v'], 'TestRestfolk_EligibleYieldAndBuildingBonus'), ('no-building', 'server/internal/economy/automatic_food.go', 'HexYieldPerWorker(opt.RatePerGood[good], opt.MultPerGood[good])', 'HexYieldPerWorker(opt.RatePerGood[good], 1)', ['tools/gotest.sh', './internal/economy', '-run', 'TestRestfolk_EligibleYieldAndBuildingBonus', '-v'], 'TestRestfolk_EligibleYieldAndBuildingBonus'), ('no-place-guard', 'server/internal/economy/automatic_food.go', 'opt.PlaceCapPerGood[good] <= 0', 'false', ['tools/gotest.sh', './internal/economy', '-run', 'TestRestfolk_EligibleYieldAndBuildingBonus', '-v'], 'TestRestfolk_EligibleYieldAndBuildingBonus'), ('recompute-no-fish', 'server/internal/economy/recompute.go', 'rawRates[GoodFish] += automatic.RemainderFish', 'rawRates[GoodFish] += 0', ['tools/gotest.sh', './internal/economy', '-run', 'TestRestfolk_ForecastStartingRequiredRecompute', '-v'], 'TestRestfolk_ForecastStartingRequiredRecompute'), ('start-no-fish', 'server/internal/economy/founding_placement.go', 'AutomaticFoodForPopulation(hexOptions, population).FoodPerTick()', 'AutomaticFoodForPopulation(hexOptions, population).GrainPerTick()', ['tools/gotest.sh', './internal/economy', '-run', 'TestRestfolk_ForecastStartingRequiredRecompute', '-v'], 'TestRestfolk_ForecastStartingRequiredRecompute'), ('required-no-fish', 'server/internal/economy/founding_placement.go', 'guaranteedFood := automatic.FoodPerTick()', 'guaranteedFood := automatic.GrainPerTick()', ['tools/gotest.sh', './internal/economy', '-run', 'TestRestfolk_ForecastStartingRequiredRecompute', '-v'], 'TestRestfolk_ForecastStartingRequiredRecompute'), ('forecast-no-fish', 'server/internal/economy/founding_forecast.go', 'fishProd := automatic.RemainderFish', 'fishProd := 0.0', ['tools/gotest.sh', './internal/economy', '-run', 'TestRestfolk_ForecastStartingRequiredRecompute', '-v'], 'TestRestfolk_ForecastStartingRequiredRecompute'), ('dryrun-commits', 'server/cmd/recompute-all/main.go', 'err = tx.Rollback(ctx)', 'err = tx.Commit(ctx)', ['tools/gotest.sh', './cmd/recompute-all', '-run', 'TestRecomputeDryRun_PreviewsAndRollsBack', '-v'], 'TestRecomputeDryRun_PreviewsAndRollsBack')]
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
