from pathlib import Path
import subprocess,os,json
w=Path('/tmp/megaron-hex-units-mutants');out=Path('/tmp/megaron-codex-hex-units-20261028/docs/reviews/hex-units/mutants');p=w/'web/static/js/megaron/render/map.js';base=p.read_text();old="chooseMarchTarget(b.dataset[verb==='march'?'hexMarch':'hexRedirect'])";assert old in base;p.write_text(base.replace(old,'chooseMarchTarget(null)',1));env={**os.environ,'HEX_ACTION_ONLY':'1'}
try:
 with (out/'redirect-selection-red.log').open('w') as f:r=subprocess.run(['python3','tools/hex_units_bild_20261028.py',str(out/'redirect-selection-red')],cwd=w,env=env,stdout=f,stderr=subprocess.STDOUT)
 assert r.returncode!=0
 assert 'assert selected==march' in (out/'redirect-selection-red.log').read_text()
finally:p.write_text(base)
with (out/'redirect-selection-restored.log').open('w') as f:r=subprocess.run(['python3','tools/hex_units_bild_20261028.py',str(out/'redirect-selection-restored')],cwd=w,env=env,stdout=f,stderr=subprocess.STDOUT)
assert r.returncode==0
p=out/'proof.json';d=json.loads(p.read_text());d.append({'mutant':'redirect-selection-native','red':True,'restored_green':True});p.write_text(json.dumps(d,indent=2));print('native redirect selection red -> green',flush=True)
