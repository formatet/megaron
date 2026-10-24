#!/usr/bin/env python3
"""Reintroduce integration review's two bugs and require named behavioral failures.
Run only in the owned integration worktree during an exclusive source window.
"""
from pathlib import Path
from datetime import datetime
import hashlib,json,subprocess
ROOT=Path(__file__).resolve().parents[1]
OUT=ROOT/'docs/reviews/correspondence-integration/mutations'
OUT.mkdir(parents=True,exist_ok=True)
case_original="""CASE WHEN sender_id=$2 AND NOT returned AND trade_offer IS NOT NULL THEN
 (SELECT COALESCE(jsonb_object_agg(field.key,field.value),'{}'::jsonb)
  FROM jsonb_each(trade_offer) field
  WHERE field.key IN ('kind','want_good','want_qty','offer_silver','offer_good','offer_qty','want_silver'))
 || jsonb_build_object('status',CASE WHEN trade_offer->>'status'='cancelled' THEN 'cancelled' ELSE 'pending' END)
 ELSE trade_offer END"""
case_mutant="""CASE WHEN sender_id=$2 AND NOT returned AND trade_offer IS NOT NULL AND trade_offer->>'status'<>'cancelled' THEN jsonb_set(trade_offer,'{status}','"pending"') ELSE trade_offer END"""
results=[]
for name,path,old,new,test,failure in [
 ('case-status-mask-leaks-outcome','server/api/handlers/correspondence_cases.go',case_original,case_mutant,'TestCorrespondencePreReturnOfferContainsOnlyOriginalTerms','pre-return archive leaked resolution fields'),
 ('completed-trade-disappears','server/api/handlers/correspondence_memory.go','AND l.accepted_xid IS NOT NULL',"AND l.trade_offer->>'status'='accepted' AND l.accepted_xid IS NOT NULL",'TestCorrespondenceAcceptedMemorySurvivesActualCompletedTrade','completed accepted agreement disappeared'),
 ('historical-completed-acceptance-lost','server/db/migrations/209_correspondence_memory.up.sql',"WHERE trade_offer->>'status' IN ('accepted','returned');","WHERE trade_offer->>'status'='accepted';",'TestCorrespondenceUpgradeRetainsCompletedAcceptanceWithoutInventingTime','historical returned: acceptance=false'),
]:
 p=ROOT/path;original=p.read_bytes();assert original.decode().count(old)==1,name
 sha=hashlib.sha256(original).hexdigest();start=datetime.now().astimezone().isoformat()
 try:
  p.write_text(original.decode().replace(old,new,1))
  run=subprocess.run([str(ROOT/'tools/gotest.sh'),'./api/handlers','-run','^'+test+'$'],cwd=ROOT,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
  (OUT/(name+'-red.log')).write_text(run.stdout)
  assert run.returncode!=0 and ('--- FAIL: '+test) in run.stdout and failure in run.stdout,name+' did not fail named behavior'
 finally:p.write_bytes(original)
 assert hashlib.sha256(p.read_bytes()).hexdigest()==sha
 results.append(dict(name=name,source=path,sha256=sha,started=start,restored=datetime.now().astimezone().isoformat(),result='named RED / byte-exact restored'))
run=subprocess.run([str(ROOT/'tools/gotest.sh'),'./api/handlers','-run','^TestCorrespondence'],cwd=ROOT,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
(OUT/'restored-correspondence.log').write_text(run.stdout);assert run.returncode==0,run.stdout
(OUT/'mutations.json').write_text(json.dumps(results,indent=2))
print('Three integration source bugs named RED; exact restore and fresh correspondence tests GREEN')
