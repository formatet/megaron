#!/usr/bin/env python3
"""Restore a composer physically; require native Firefox DOM red, restore green."""
import pathlib,subprocess
ROOT=pathlib.Path(__file__).resolve().parents[1];OUT=ROOT/'docs/reviews/offer-en-knapp'
p=ROOT/'web/static/js/megaron/ui/drawers/kult.js';original=p.read_bytes();source=original.decode()
needle="    html += await renderLockedActions('cult');";assert source.count(needle)==1
mutant=source.replace(needle,"    html += '<details class=\"offer-composer\"><summary>Compose your own offering</summary></details>';\n"+needle)
try:
 p.write_text(mutant)
 result=subprocess.run(['python3','tools/offer_proof.py','check'],cwd=ROOT,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True)
 (OUT/'mutation-dom.log').write_text(result.stdout)
 assert result.returncode!=0 and 'composed offering leaked into DOM' in result.stdout,result.stdout
 print('composer in native DOM: named red')
finally:p.write_bytes(original)
result=subprocess.run(['python3','tools/offer_proof.py','check'],cwd=ROOT,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True)
(OUT/'restored-dom.log').write_text(result.stdout);assert result.returncode==0,result.stdout
assert p.read_bytes()==original
print('byte restored native DOM green')
