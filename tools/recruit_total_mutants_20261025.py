#!/usr/bin/env python3
"""Fresh-DB physical mutations of recruitment projection and reservation gates."""
import json,subprocess,os,socket
from pathlib import Path
root=Path(__file__).resolve().parents[1]
out=root/'docs/reviews/rekrytering-total';out.mkdir(parents=True,exist_ok=True)
quote='server/api/handlers/recruit_quote.go'
cases=[
 ('population',quote,'population <= batch','population < batch','TestRecruitQuoteMatchesPopulation'),
 ('building',quote,'b.needed && !buildings[b.name]','false && b.needed && !buildings[b.name]','TestRecruitQuoteNamesEveryBlockingGate/building'),
 ('whole-batch',quote,'need := spec.Costs[g] * float64(batch)','need := spec.Costs[g]','TestRecruitQuoteNamesEveryBlockingGate/goods'),
 ('cedar',quote,'have += math.Max(0, stock["cedar"]-spec.Costs["cedar"]*float64(batch))','have += 0 * math.Max(0, stock["cedar"]-spec.Costs["cedar"]*float64(batch))','TestRecruitQuoteNamesEveryBlockingGate/cedar'),
 ('queue',quote,'pendingShips >= 10','pendingShips > 10','TestRecruitQuoteNamesEveryBlockingGate/queue'),
 ('refund-actual-goods','server/api/handlers/province.go','unitID, paidCosts, totalKharis, crew','unitID, totalCosts, totalKharis, crew','TestRecruitCedarReceiptAndCancellation'),
]
results=[]
for name,file,old,new,test in cases:
 p=root/file;original=p.read_bytes();s=original.decode();assert s.count(old)==1,(name,s.count(old))
 try:
  p.write_text(s.replace(old,new))
  with socket.socket() as listener:
   listener.bind(('127.0.0.1',0));port=listener.getsockname()[1]
  run=subprocess.run([str(root/'tools/gotest.sh'),'./api/handlers/','-run',test],cwd=root,capture_output=True,text=True,env={**os.environ,'GOTEST_PORT':str(port)})
  log=run.stdout+run.stderr;(out/(name+'-mutant.log')).write_text(log)
  assert run.returncode!=0 and '--- FAIL: TestRecruit' in log and '[build failed]' not in log,(name,log)
  results.append({'name':name,'test':test,'exit':run.returncode,'assertion_red':True})
 finally:
  p.write_bytes(original);assert p.read_bytes()==original
(out/'mutations.json').write_text(json.dumps(results,indent=2)+'\n')
print('PASS six assertion-red mutations, exact restore')
