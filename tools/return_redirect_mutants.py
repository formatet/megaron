#!/usr/bin/env python3
"""Physical return redirect mutants, each on a fresh DB, in a separate clean checkout.
Usage: python3 tools/return_redirect_mutants.py CHECKOUT OUTPUT
Requires assertion-red (not build failure), exact restoration, then fresh green.
"""
from pathlib import Path
import hashlib,json,subprocess,sys,os,socket
checkout=Path(sys.argv[1]).resolve();out=Path(sys.argv[2]).resolve();out.mkdir(parents=True,exist_ok=True)
assert not subprocess.check_output(['git','-C',str(checkout),'status','--porcelain'],text=True).strip(),'use a clean separate checkout'
source=subprocess.check_output(['git','-C',str(checkout),'rev-parse','HEAD'],text=True).strip()
cases=[
 ('return-intent','server/internal/combat/recall_redirect.go','if o.Mode == "redirect" && marchIntent != nil && *marchIntent == "explore_return" {','if false && o.Mode == "redirect" && marchIntent != nil && *marchIntent == "explore_return" {',0,'TestRedirectExpeditionReturnArrivesAtActualDestination'),
 ('home-anchor','server/internal/combat/recall_redirect.go','newIntent, newColonyName, homeSettlementID = nil, nil, nil','newIntent, newColonyName, homeSettlementID = nil, nil, homeSettlementID',0,'TestRedirectExpeditionReturnArrivesAtActualDestination'),
 ('naval-exemption','server/db/migrations/217_correct_remote_land_garrisons.up.sql',"u.category='land' AND ",'',0,'TestRemoteLandGarrisonCorrectionPreservesPhysicalState'),
 ('correction-audit','server/db/migrations/217_correct_remote_land_garrisons.up.sql','UnitGarrisonLocationCorrectedV1','UnitGarrisonLocationWrongV1',0,'TestRemoteLandGarrisonCorrectionPreservesPhysicalState'),
]
def test_env():
 with socket.socket() as sock:
  sock.bind(('127.0.0.1',0));port=sock.getsockname()[1]
 return {**os.environ,'GOTEST_PORT':str(port)}
manifest=[]
for name,rel,old,new,nth,expected in cases:
 p=checkout/rel;original=p.read_bytes();s=original.decode();positions=[];start=0
 while True:
  pos=s.find(old,start)
  if pos<0:break
  positions.append(pos);start=pos+len(old)
 assert len(positions)>nth,(name,positions)
 pos=positions[nth];mutated=s[:pos]+new+s[pos+len(old):]
 try:
  p.write_text(mutated)
  result=subprocess.run([str(checkout/'tools/gotest.sh'),'./internal/combat','-run','TestRedirectExpeditionReturn|TestRemoteLandGarrison|TestRecallExpedition|TestRecallPlainAndExpeditionRedirect'],cwd=checkout,env=test_env(),text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
  (out/(name+'.log')).write_text(result.stdout)
  assert result.returncode and '--- FAIL: '+expected in result.stdout and '[build failed]' not in result.stdout,(name,result.stdout[-2000:])
  manifest.append({'name':name,'file':rel,'old':old,'new':new,'occurrence':nth,'expected_test':expected,'assertion_red':True,'source_commit':source,'source_sha256':hashlib.sha256(original).hexdigest()})
  print('assertion-red',name,flush=True)
 finally:
  p.write_bytes(original);assert p.read_bytes()==original
result=subprocess.run([str(checkout/'tools/gotest.sh'),'./internal/combat','-run','TestRedirectExpeditionReturn|TestRemoteLandGarrison|TestRecallExpedition|TestRecallPlainAndExpeditionRedirect'],cwd=checkout,env=test_env(),text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
(out/'restored-green.log').write_text(result.stdout);assert result.returncode==0,result.stdout
(out/'verified.json').write_text(json.dumps(manifest,indent=2)+'\n')
assert not subprocess.check_output(['git','-C',str(checkout),'status','--porcelain'],text=True).strip()
print('PASS',len(cases),'assertion-red mutants, exact restore, fresh redirect/recall green')
