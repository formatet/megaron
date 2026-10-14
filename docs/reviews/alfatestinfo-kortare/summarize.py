#!/usr/bin/env python3
"""Summarize actual DOM word counts and layout from two independent Firefox runs."""
import hashlib,json,pathlib
HERE=pathlib.Path(__file__).resolve().parent;ROOT=HERE.parents[2]
a=json.loads((HERE/'after/verification.json').read_text());b=json.loads((HERE/'after-repeat/verification.json').read_text());old=json.loads((HERE/'before/verification.json').read_text())
assert [(x['words'],x['metrics']) for x in a['captures']]==[(x['words'],x['metrics']) for x in b['captures']]
for proof in (a,b):
 assert not proof['errors']
 for c in proof['captures']:
  assert c['metrics']['width']<=c['metrics']['viewport']+1
  assert len(c['images'])==4 and all(i['width']==1440 and i['height']==1000 for i in c['images'])
result={'before_words':old['captures'][0]['words'],'after_words':b['captures'][0]['words'],'reduction_pct':100*(1-b['captures'][0]['words']/old['captures'][0]['words']),'before_process':old['health'],'after_process':a['health'],'repeat_process':b['health'],'desktop_mobile_no_horizontal_overflow':True,'repeat_words_and_metrics_identical':True,'assets':{n:hashlib.sha256((ROOT/'web/static/alfatest'/(n+'.png')).read_bytes()).hexdigest() for n in ('karta','stad','produktion','handel')}}
(HERE/'comparison.json').write_text(json.dumps(result,indent=2)+'\n');print(json.dumps(result,indent=2))
