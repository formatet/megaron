#!/usr/bin/env python3
"""Physical reference-target mutation; original source restored byte for byte."""
import hashlib,os,pathlib,re,subprocess
HERE=pathlib.Path(__file__).resolve().parent;ROOT=HERE.parents[2]
source=ROOT/'server/internal/world/mapgen.go';original=source.read_bytes()
changed,n=re.subn(rb'(silverSourcesAtReference\s*=\s*)10',rb'\g<1>4',original)
assert n==1
clean={k:os.environ[k] for k in ('HOME','PATH')}
try:
 source.write_bytes(changed)
 r=subprocess.run(['go','test','-count=1','./internal/world','-run','^TestGenerateMapTenSilverSourcesOn64x64$'],cwd=ROOT/'server',env=clean,text=True,capture_output=True)
 (HERE/'mutation-four-red.log').write_text(r.stdout+r.stderr)
 assert r.returncode!=0 and '4 silver hexes, want 10' in r.stdout,r
finally:source.write_bytes(original)
assert source.read_bytes()==original
r=subprocess.run(['go','test','-count=1','./internal/world','-run','TestGenerateMapTenSilverSourcesOn64x64|TestSilverSourceTargetScalesWithMapArea'],cwd=ROOT/'server',env=clean,text=True,capture_output=True)
(HERE/'mutation-restored-green.log').write_text(r.stdout+r.stderr)
assert r.returncode==0,r
print('RED reference10 -> 4, TestGenerateMapTenSilverSourcesOn64x64; BYTE RESTORED; fresh GREEN both named tests; source sha256',hashlib.sha256(original).hexdigest())
