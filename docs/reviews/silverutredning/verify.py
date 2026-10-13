#!/usr/bin/env python3
"""Independent geometry/anchor check; optional complete isolated-run comparison."""
import hashlib,json,math,pathlib,statistics,sys
OUT=pathlib.Path(sys.argv[1]) if len(sys.argv)>1 else pathlib.Path(__file__).parent/'results'
summary=json.loads((OUT/'summary.json').read_text())
def distance(a,b):return (abs(a[0]-b[0])+abs(a[1]-b[1])+abs(a[0]+a[1]-b[0]-b[1]))//2
rates={}
for target in [4,5,10,20,40]:
 maps=[json.loads(line) for line in (OUT/f'maps-{target}.jsonl').read_text().splitlines()]
 assert len(maps)==20 and [m['requested_seed'] for m in maps]==[1009*i for i in range(1,21)]
 for m in maps:
  for collection in ['candidates','hosts']:
   for point in m[collection]:
    assert point[2]==min(distance(point,s) for s in m['deposits']), 'nearest-silver geometry'
    same=[distance(point,s) for s in m['deposits'] if point[4]==s[3]]
    assert point[3]==(min(same) if same else None), 'same-landmass distance'
  assert len(m['candidates'])==len({(p[0],p[1]) for p in m['candidates']}), 'duplicate starttile'
  for i,p in enumerate(m['hosts']):
   assert any(p[:2]==c[:2] for c in m['candidates']), 'host not an eligible start'
   assert all(distance(p,h)>=5 for h in m['hosts'][:i]), 'host spacing'
  assert all(distance(a,b)>=5 for i,a in enumerate(m['deposits']) for b in m['deposits'][:i]), 'silver spacing'
 for slots in [5,10]:
  # Independent rate calculation: r0 from L1 anchor, times (1 + .7*level).
  expected={level:statistics.mean(sum(slots*(1 if s[2]=='hills' else 2.5)/1.7*(1+.7*level) for s in m['deposits']) for m in maps) for level in [1,3]}
  for level in [1,3]:
   assert math.isclose(summary[f'{target}hex-{slots}slots'][f'l{level}_mean'],expected[level]), 'worker-rate anchor L'+str(level)
rates=summary['4hex-5slots']
assert rates['l1_mean']==27.875, 'STOPP1 baseline changed'
assert (OUT/'maps-20.jsonl').read_bytes()==(OUT/'maps-40.jsonl').read_bytes(), 'saturation observation changed'
print('PASS independent axial distances for every starttile/host, same-landmass, spacing, actual rate anchors, STOPP1 baseline and saturation')
if len(sys.argv)>2:
 repeat=pathlib.Path(sys.argv[2])
 for target in [4,5,10,20,40]:
  name=f'maps-{target}.jsonl';a=(OUT/name).read_bytes();b=(repeat/name).read_bytes()
  assert a==b, 'isolated replay mismatch '+name
  print('PASS isolated replay',name,hashlib.sha256(a).hexdigest())
