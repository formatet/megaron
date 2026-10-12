#!/usr/bin/env python3
"""Each approved recipe: physical catalogue mutation, actual Offer debit red, restore."""
from pathlib import Path
import json,os,socket,subprocess
ROOT=Path(__file__).resolve().parents[1];OUT=ROOT/'docs/reviews/religion-alfa/mutations-recipes';OUT.mkdir(parents=True,exist_ok=True)
cases=[
 ('vision','{"grain": 10, "silver": 100}','{"grain": 9, "silver": 100}'),
 ('walls','{"grain": 8, "timber": 4}','{"grain": 8, "timber": 3}'),
 ('green','{"grain": 15, "silver": 50}','{"grain": 14, "silver": 50}'),
 ('thread','{"timber": 4, "silver": 100}','{"timber": 4, "silver": 99}'),
 ('passage','{"fish": 6, "silver": 100}','{"fish": 5, "silver": 100}'),
]
results=[]
p=ROOT/'server/internal/religion/prayers.go';original=p.read_bytes()
def fresh(args):
 sock=socket.socket();sock.bind(('127.0.0.1',0));port=sock.getsockname()[1];sock.close()
 return subprocess.run([str(ROOT/'tools/gotest.sh'),*args],cwd=ROOT,env=dict(os.environ,GOTEST_PORT=str(port)),text=True,capture_output=True)
for name,needle,replacement in cases:
 assert original.decode().count(needle)==1,(name,'nonunique anchor')
 try:
  p.write_text(original.decode().replace(needle,replacement,1))
  r=fresh(['./internal/religion','./api/handlers','-run','^TestAlphaTraditionalRecipesOctoberDecision$|^TestAlphaOfferDebitsDecisionRecipeExactly$'])
  (OUT/(name+'-red.log')).write_text(r.stdout+r.stderr)
  assert r.returncode!=0 and '--- FAIL: TestAlphaTraditionalRecipesOctoberDecision' in r.stdout and '--- FAIL: TestAlphaOfferDebitsDecisionRecipeExactly' in r.stdout,(name,r.stdout,r.stderr)
 finally:p.write_bytes(original)
 assert p.read_bytes()==original
 results.append({'recipe':name,'catalogue_named_red':True,'actual_default_Offer_debit_named_red':True,'byte_restored':True})
 print(name,'catalogue AND actual Offer debit named red; byte restored',flush=True)
r=fresh(['./internal/religion','./api/handlers','-run','^TestAlpha|^TestBeta']);(OUT/'restored-fresh.log').write_text(r.stdout+r.stderr);assert r.returncode==0,(r.stdout,r.stderr)
for entry in results:entry['restored_fresh_green']=True
(OUT/'results.json').write_text(json.dumps(results,indent=2)+'\n')
