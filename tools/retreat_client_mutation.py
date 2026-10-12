#!/usr/bin/env python3
"""Physically reintroduce retired percentages, require named red, restore green."""
import pathlib,subprocess
ROOT=pathlib.Path(__file__).resolve().parents[1]
p=ROOT/'web/static/js/megaron/ui/retreat.js';out=ROOT/'docs/reviews/retreat-tva-lagen'
original=p.read_bytes();source=original.decode()
needle="if (value === 'loyalty') return { by_loyalty: true };\n  return null;"
assert source.count(needle)==1
mutant=source.replace(needle,"if (value === 'loyalty') return { by_loyalty: true };\n  const f = parseFloat(value);\n  return Number.isFinite(f) ? { retreat_at_loss: f } : null;")
try:
 p.write_text(mutant)
 result=subprocess.run(['node','--test','web/static/js/megaron/ui/retreat.test.mjs'],cwd=ROOT,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True)
 (out/'mutation-client.log').write_text(result.stdout)
 assert result.returncode!=0 and 'retired retreat percentage became a request' in result.stdout,result.stdout
 print('retired percentage: named red')
finally:p.write_bytes(original)
result=subprocess.run(['node','--test','web/static/js/megaron/ui/retreat.test.mjs'],cwd=ROOT,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True)
(out/'restored-client.log').write_text(result.stdout);assert result.returncode==0,result.stdout
assert p.read_bytes()==original
print('client byte restored green')
