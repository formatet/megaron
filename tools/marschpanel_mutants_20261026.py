#!/usr/bin/env python3
"""Run assertion mutations in a separate clean worktree; restore exact file bytes."""
import argparse,hashlib,json,os,socket,subprocess
from pathlib import Path
ROOT=Path(__file__).resolve().parents[1]
p=argparse.ArgumentParser();p.add_argument('checkout',type=Path);p.add_argument('--only');a=p.parse_args();CHECK=a.checkout.resolve()
assert CHECK!=ROOT
assert not subprocess.check_output(['git','-C',str(CHECK),'status','--porcelain']).strip()
OUT=ROOT/'docs/reviews/marschpanel/mutations';OUT.mkdir(exist_ok=True)
J='web/static/js/megaron/ui/marchctx.js'
G='server/cmd/keryx/'
CASES=[
('web-default',J,'index === 0 && group.ids.length ? 1 : 0','index === 0 && group.ids.length ? group.ids.length : 0','js','H: defaults select one'),
('web-selection',J,'if(selected(i,k))picks.push','if(selected(i,0))picks.push','js','named choices let the third'),
('web-preflight',J,"p.order.intent==='explore'&&p.mode!=='redirect'","false&&p.order.intent==='explore'&&p.mode!=='redirect'",'js','too-short expedition is refused'),
('web-version',J,'selectionVersion!==marchSelectionVersion||world!==State.WORLD_ID||destination!==State.marchCtxDest','world!==State.WORLD_ID||destination!==State.marchCtxDest','js','changing the named selection'),
('keryx-name',G+'cmd_resolve.go','strings.EqualFold(strings.TrimSpace(u.DisplayName), needle) || ','','./cmd/keryx/','TestResolveUnitExactDisplayName'),
('keryx-preflight',G+'cmd_unit.go','if intent == "explore" {\n\t\t\t\tif _, err := fetchMarchPreview','if false && intent == "explore" {\n\t\t\t\tif _, err := fetchMarchPreview','./cmd/keryx/','TestExploreRefusalBeforeRunnerDispatch'),
('keryx-query',G+'cmd_march_preview.go','query.Set(key, fmt.Sprint(value))','query.Set(key, fmt.Sprint(value)+"0")','./cmd/keryx/','TestExploreTicksOrderAndPreview'),
('api-mask','server/api/handlers/unit_march_preview.go','if rej.Code == combat.RejectExpeditionBudget {','if false && rej.Code == combat.RejectExpeditionBudget {','./api/handlers/','TestMarchPreview_ReadOnlyAndTiming'),
('outbound-code','server/internal/combat/march_start.go','return nil, rejectExpeditionBudget("the chosen hex','return nil, reject(http.StatusUnprocessableEntity, "the chosen hex','./api/handlers/','TestMarchPreview_ReadOnlyAndTiming'),
('return-budget','server/internal/combat/march_start.go','if travelTicks+returnTicks > expeditionPlan.LengthTicks {','if false && travelTicks+returnTicks > expeditionPlan.LengthTicks {','./api/handlers/','TestMarchPreview_ReadOnlyAndTiming'),
]
results=[]
for name,file,old,new,pkg,test in CASES:
 if a.only and name!=a.only:continue
 target=CHECK/file;original=target.read_bytes();source=original.decode();assert source.count(old)==1,(name,source.count(old))
 try:
  target.write_text(source.replace(old,new))
  if pkg=='js':cmd=['node','--test','web/static/js/megaron/ui/marchctx.test.mjs']
  else:cmd=[str(CHECK/'tools/gotest.sh'),pkg,'-run',test,'-count=1']
  with socket.socket() as s:s.bind(('127.0.0.1',0));port=s.getsockname()[1]
  r=subprocess.run(cmd,cwd=CHECK,capture_output=True,text=True,env={**os.environ,'GOTEST_PORT':str(port)})
  log=r.stdout+r.stderr;(OUT/(name+'.log')).write_text(log)
  red=r.returncode!=0 and ('--- FAIL:' in log or 'not ok' in log) and '[build failed]' not in log
  results.append(dict(name=name,file=file,old=old,new=new,test=test,command=cmd,assertion_red=red,source_sha256=hashlib.sha256(original).hexdigest()))
  print(name,'assertion RED' if red else 'NOT PROVEN',flush=True)
 finally:
  target.write_bytes(original);assert target.read_bytes()==original
 (OUT/('verified-'+a.only+'.json' if a.only else 'verified.json')).write_text(json.dumps(results,indent=2)+'\n')
assert all(r['assertion_red'] for r in results)
print('PASS all mutations; exact byte restore')
