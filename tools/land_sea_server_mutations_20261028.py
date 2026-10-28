#!/usr/bin/env python3
"""Physical assertion-red land/sea mutations in an isolated final checkout.
Usage: land_sea_server_mutations_20261028.py CHECKOUT OUTPUT
Each run uses a separately fresh DB and a free-selected port. Exact original
bytes are restored in finally; the final matrix must pass on another fresh DB.
"""
from pathlib import Path
import hashlib,json,os,socket,subprocess,sys
root=Path(sys.argv[1]).resolve();out=Path(sys.argv[2]).resolve();out.mkdir(parents=True,exist_ok=True)
def git(*args):return subprocess.check_output(['git','-C',str(root),*args],text=True).strip()
assert not git('status','--porcelain'),'isolated checkout must start clean'
commit=git('rev-parse','HEAD')
# name, file, old, new, occurrence (1 based), package, named assertion test
cases=[
 ('schema-land-choice','server/db/migrations/218_transport_route_choice.up.sql',"CHECK(route_mode IN ('','land','sea'))","CHECK(route_mode IN ('','sea'))",1,'./api/handlers/','TestStandingOrderChoiceCreateListAndUnavailableReject'),
 ('resolver-forgets-explicit-mode','server/internal/province/trade_route.go','if by == "" {','if true {',1,'./api/handlers/','TestTransferExplicitLandDoesNotChooseAvailableSea'),
 ('transport-forgets-choice','server/internal/transport/transport.go','raw, p.DepartedTick, p.By,','raw, p.DepartedTick, "",',1,'./api/handlers/','TestTransferChoiceStrictnessAndPersistence'),
 ('transport-allows-category-substitution','server/internal/transport/transport.go','if (p.By == "land" && p.Category != "land") || (p.By == "sea" && p.Category != "naval") {','if false {',1,'./internal/transport/','TestDispatchExplicitChoiceRejectsCategorySubstitution'),
 ('transfer-ignores-choice','server/api/handlers/province.go','province.MapPosition{Q: destQ, R: destR}, req.By,','province.MapPosition{Q: destQ, R: destR}, "",',1,'./api/handlers/','TestTransferExplicitLandDoesNotChooseAvailableSea'),
 ('preview-wrong-wall-clock','server/api/handlers/trade_preview.go','o["arrives_at"] = at','o["arrives_at"] = now',1,'./api/handlers/','TestTransportOptionsBothModesAnchoredAndReadOnly'),
 ('preview-consumes-prayer','server/api/handlers/trade_preview.go','defer tx.Rollback(r.Context())','defer tx.Commit(r.Context())',1,'./api/handlers/','TestTransportOptionsBothModesAnchoredAndReadOnly'),
 ('offer-forgets-choice','server/api/handlers/messenger.go','"by":          req.TradeOffer.By,','"by":          "",',1,'./api/handlers/','TestAgreementChoicePersistsAndControlsAcceptedDispatch'),
 ('accept-falls-back-on-selected-sea','server/api/handlers/messenger.go','if routeBy == "sea" {','if routeBy == "land" {',1,'./api/handlers/','TestAgreementExplicitSeaNeverFallsBackAtAccept'),
 ('standing-create-forgets-choice','server/api/handlers/standing_order.go','req.CrewedBySettlement, req.By,','req.CrewedBySettlement, "",',1,'./api/handlers/','TestStandingOrderChoiceCreateListAndUnavailableReject'),
 ('standing-create-skips-selected-path','server/api/handlers/standing_order.go','if req.By != "" {','if false {',1,'./api/handlers/','TestStandingOrderChoiceCreateListAndUnavailableReject'),
 ('standing-outbound-ignores-choice','server/internal/combat/standing_orders.go','province.MapPosition{Q: toQ, R: toR}, o.by)','province.MapPosition{Q: toQ, R: toR}, "")',1,'./internal/combat/','TestStandingOrderExplicitChoiceControlsSweepWithoutFallback'),
 ('standing-return-forgets-choice','server/internal/combat/standing_orders.go','transport.DispatchParams{By: o.by,','transport.DispatchParams{By: "",',2,'./internal/combat/','TestStandingOrderExplicitChoiceControlsSweepWithoutFallback'),
 ('agreement-return-forgets-choice','server/internal/economy/trade.go','leg2ShipUnitID, saved, departure, leg1By,','leg2ShipUnitID, saved, departure, "",',1,'./api/handlers/','TestAgreementChoicePersistsAndControlsAcceptedDispatch'),
 ('empty-hull-return-forgets-choice','server/internal/economy/trade.go','*shipUnitID, raw, currentTick, routeBy,','*shipUnitID, raw, currentTick, "",',1,'./api/handlers/','TestTransferChoiceStrictnessAndPersistence'),
]
files={f:(root/f).read_bytes() for _,f,*_ in cases}
def hashes():return {f:hashlib.sha256((root/f).read_bytes()).hexdigest() for f in files}
before=hashes()
def fresh(args):
 with socket.socket() as s:s.bind(('127.0.0.1',0));port=s.getsockname()[1]
 env={k:os.environ[k] for k in ('HOME','PATH','GOPATH','GOCACHE') if k in os.environ};env['GOTEST_PORT']=str(port)
 res=subprocess.run([str(root/'tools/gotest.sh'),*args],cwd=root,env=env,capture_output=True,text=True)
 return res,port
proof=[]
try:
 for name,file,old,new,nth,pkg,test in cases:
  p=root/file;original=files[file];text=original.decode();assert text.count(old)>=nth,(name,old)
  start=-1
  for _ in range(nth):start=text.index(old,start+1)
  try:
   p.write_text(text[:start]+new+text[start+len(old):])
   res,port=fresh([pkg,'-run','^'+test+'$']);log=res.stdout+res.stderr;(out/(name+'-red.log')).write_text(log)
   assert res.returncode and '--- FAIL: '+test in log,(name,log)
   assert '[build failed]' not in log and 'panic:' not in log and 'fatal error:' not in log,(name,log)
   proof.append({'name':name,'file':file,'old':old,'new':new,'occurrence':nth,'package':pkg,'named_test':test,'assertion_red':True,'port':port})
  finally:p.write_bytes(original)
  assert hashes()==before,'restoration not byte-identical'
  print('RED + exact restore: '+name,flush=True)
finally:
 for file,data in files.items():(root/file).write_bytes(data)
assert hashes()==before and not git('status','--porcelain')
names=sorted({c[-1] for c in cases})
res,port=fresh(['./api/handlers/','./internal/combat/','./internal/transport/','-run','^('+'|'.join(names)+')$'])
(out/'restored-green.log').write_text(res.stdout+res.stderr);assert res.returncode==0,res.stdout+res.stderr
assert hashes()==before and not git('status','--porcelain')
(out/'proof.json').write_text(json.dumps({'commit':commit,'mutations':proof,'source_sha256':before,'byte_identical_restore':True,'fresh_restored_green':True,'green_port':port},indent=2)+'\n')
print('STABLE: fifteen assertion-red core mutants, exact restore and fresh restored green',flush=True)
