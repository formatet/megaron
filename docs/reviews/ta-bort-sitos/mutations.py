#!/usr/bin/env python3
"""Compiling physical mutation -> named semantic RED -> exact restore -> fresh GREEN."""
from pathlib import Path
import hashlib,os,socket,subprocess,sys
ROOT=Path(__file__).resolve().parents[3];OUT=Path(__file__).resolve().parent
cases=[
 ('return-food','server/db/migrations/184_remove_sitos.up.sql','SET amount = EXCLUDED.amount','SET amount = 0',['./internal/economy','TestFoodStore_MigrationConservesLazyStockAndMissingGood']),
 ('raise-ceiling','server/db/migrations/184_remove_sitos.up.sql','cap = EXCLUDED.cap','cap = 5',['./internal/economy','TestFoodStore_MigrationConservesLazyStockAndMissingGood']),
 ('missing-good','server/db/migrations/184_remove_sitos.up.sql','LEFT JOIN settlement_goods sg','JOIN settlement_goods sg',['./internal/economy','TestFoodStore_MigrationConservesLazyStockAndMissingGood']),
 ('retired-tick','server/internal/events/retired_tick.go','return nil','return context.Canceled',['./internal/events','TestRetiredSitosTick_CompletesWithoutReschedule']),
 ('no-reseed','server/cmd/server/main.go','\n\t\tevents.ScheduledFoodTick,','\n\t\tevents.ScheduledFoodTick, events.ScheduledSitosTick,',['./cmd/server','TestSeedDailyTicks_NoRetiredReserve']),
 ('food-api','server/api/handlers/province.go','"food": map[string]any{','"retired_food": map[string]any{',['./api/handlers','TestSettlementsOverviewParity_MatchesProvinceGet']),
 ('retired-loot-query','server/internal/combat/occupation.go','allowed := map[string]bool{}','if _, err := tx.Exec(ctx, `SELECT 1 FROM settlement_granary`); err != nil { return nil, err }; allowed := map[string]bool{}',['./internal/combat','TestExecuteOccupyAction_Sack']),
]
if len(sys.argv)>1:cases=cases[int(sys.argv[1]):]
for name,relative,old,new,(pkg,named) in cases:
 p=ROOT/relative;original=p.read_bytes();s=original.decode();assert s.count(old)==1,(name,s.count(old))
 def run(label,green):
  env={k:os.environ[k] for k in ('HOME','PATH')}
  with socket.socket() as sock:sock.bind(('127.0.0.1',0));env['GOTEST_PORT']=str(sock.getsockname()[1])
  r=subprocess.run(['tools/gotest.sh',pkg,'-run',named,'-v'],cwd=ROOT,env=env,capture_output=True,text=True);log=r.stdout+r.stderr;(OUT/(name+'-'+label+'.log')).write_text(log)
  if green:assert r.returncode==0,(name,log[-5000:])
  else:assert r.returncode and '--- FAIL: '+named in log and '[build failed]' not in log,(name,log[-5000:])
 try:p.write_text(s.replace(old,new));run('red',False)
 finally:p.write_bytes(original)
 run('restored',True);assert p.read_bytes()==original
 print(name,'named RED -> exact bytes -> fresh GREEN',hashlib.sha256(original).hexdigest(),flush=True)
