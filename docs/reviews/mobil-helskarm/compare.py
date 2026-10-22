#!/usr/bin/env python3
"""Verify independent repeats and compare baseline/fix pixels at desktop width."""
import hashlib,json
from pathlib import Path
from PIL import Image,ImageChops
out=Path(__file__).resolve().parent
result={}
for arm in ('baseline-firefox','firefox'):
 rows=json.loads((out/(arm+'.json')).read_text())
 scenes=[r for r in rows if 'scene' in r]
 assert len(scenes)==32
 for row in scenes:
  other=next(r for r in scenes if r['width']==row['width'] and r['scene']==row['scene'] and r['run']!=row['run'])
  assert row['rgba_sha256']==other['rgba_sha256']
 assert len([r for r in rows if 'page_errors' in r and not r['page_errors']])==4
 result[arm]={'identical_scene_pairs':16,'clean_contexts':4}
result['desktop']={}
for name in ('city','hex','march','drawer','notice','account','search','codex'):
 a=Image.open(out/f'firefox-1000-baseline-{name}.png').convert('RGB')
 b=Image.open(out/f'firefox-1000-{name}.png').convert('RGB')
 diff=ImageChops.difference(a,b)
 count=sum(p!=(0,0,0) for p in diff.getdata())
 result['desktop'][name]={'changed_pixels':count,'bbox':diff.getbbox()}
 assert count==0,(name,count)
print(json.dumps(result,indent=2))
