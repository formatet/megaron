#!/usr/bin/env python3
"""Physical compiling mutations, named assertion RED, byte restoration, GREEN."""
from pathlib import Path
import hashlib,os,socket,subprocess,sys
ROOT=Path(__file__).resolve().parents[3];OUT=Path(__file__).resolve().parent
cli=lambda name:['go','-C','server','test','./cmd/keryx','-run',name,'-count=1','-v']
api=['tools/gotest.sh','./api/handlers','-run','TestAIRound4','-v']
econ=['tools/gotest.sh','./internal/economy','-run','TestAIRound4','-v']
js=['node','--test','web/static/js/megaron/ui/drawers/city_production.test.mjs']
cases=[
 ('automatic-grain', 'server/internal/economy/automatic_food.go', '(potential / REF_LABOR) * float64(people)', '(potential / REF_LABOR) * 0', econ, 'TestAIRound4_Scheria1042FoodRequirement'),
 ('province-auto-wire', 'server/api/handlers/province.go', '"automatic_food":', '"automatic_food_missing":', api, 'TestAIRound4_AutomaticFoodAPIAndFixedQueue'),
 ('overview-auto-wire', 'server/api/handlers/settlement.go', 'json:"automatic_food"', 'json:"automatic_food_missing"', api, 'TestAIRound4_AutomaticFoodAPIAndFixedQueue'),
 ('cli-auto-food', 'server/cmd/keryx/cmd_gubbe.go', 'sett["automatic_food"]', 'sett["automatic_food_missing"]', cli('TestAIRound4_AutomaticFoodAndNoMoveAdvice'), 'TestAIRound4_AutomaticFoodAndNoMoveAdvice'),
 ('speculative-move', 'server/cmd/keryx/cmd_status.go', 'needed for people on best food slots · %d placed)', 'needed for people on best food slots · %d placed; could move)', cli('TestAIRound4_AutomaticFoodAndNoMoveAdvice'), 'TestAIRound4_AutomaticFoodAndNoMoveAdvice'),
 ('net-threshold', 'server/cmd/keryx/cmd_unit.go', 'if math.Abs(v) < 0.05 {', 'if math.Abs(v) < 0.5 {', cli('TestAIRound4_NetThresholdAndSingular'), 'TestAIRound4_NetThresholdAndSingular'),
 ('citizen-singular', 'server/cmd/keryx/cmd_gubbe.go', 'if n == 1 {', 'if false {', cli('TestAIRound4_NetThresholdAndSingular'), 'TestAIRound4_NetThresholdAndSingular'),
 ('hex-staff', 'server/cmd/keryx/cmd_staff.go', 'h.Building != nil && h.Building.Type == buildingType', 'h.Building != nil && false', cli('TestAIRound4_HexStaffRedirect'), 'TestAIRound4_HexStaffRedirect'),
 ('hex-ordinal', 'server/cmd/keryx/cmd_gubbe.go', 'if h.HexOrdinal == ordinal {', 'if h.HexOrdinal == -ordinal {', cli('TestAIRound4_HexSelectors'), 'TestAIRound4_HexSelectors'),
 ('place-hex-flag', 'server/cmd/keryx/cmd_place.go', 'target := hexFlag', 'target := ""', cli('TestAIRound4_HexSelectors'), 'TestAIRound4_HexSelectors'),
 ('queue-refusal', 'server/api/handlers/province.go', 'fixed at %d concurrent buildings', '%d concurrent buildings', api, 'TestAIRound4_AutomaticFoodAPIAndFixedQueue'),
 ('queue-listing', 'server/cmd/keryx/cmd_build.go', 'fixed limit; population and buildings do not add slots.', 'limit.', cli('TestAIRound4_QueueExplainsFixedSlots'), 'TestAIRound4_QueueExplainsFixedSlots'),
 ('web-automatic-food', 'web/static/js/megaron/ui/drawers/city_production.js', 'const a = pd.automatic_food;', 'const a = null;', js, 'AIRound4'),
]
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
