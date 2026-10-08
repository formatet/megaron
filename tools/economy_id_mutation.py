#!/usr/bin/env python3
"""Restore the old wrong lookup physically; require named red then restored green.
Usage: python3 tools/economy_id_mutation.py OUT
"""
from pathlib import Path
import os,subprocess,sys
ROOT=Path(__file__).resolve().parents[1];OUT=Path(sys.argv[1]);OUT.mkdir(parents=True,exist_ok=True)
source=ROOT/'web/static/js/megaron/ui/drawers/economy.js'
test=source.with_name('economy_id.test.mjs');original=source.read_text()
needle='overviewByID.get(s.settlement_id)';assert original.count(needle)==1
try:
 source.write_text(original.replace(needle,'overviewByID.get(s.id)'))
 result=subprocess.run(['node','--test',str(test)],cwd=ROOT,env={k:os.environ[k] for k in ('HOME','PATH')},capture_output=True,text=True)
 (OUT/'id-red.log').write_text(result.stdout+result.stderr)
 assert result.returncode and 'settlement overview must join settlement id' in result.stdout,result.stdout
finally:source.write_text(original)
result=subprocess.run(['node','--test',str(test)],cwd=ROOT,env={k:os.environ[k] for k in ('HOME','PATH')},capture_output=True,text=True)
(OUT/'id-restored.log').write_text(result.stdout+result.stderr);assert result.returncode==0,result.stdout
print('actual drawer lookup: named red -> restored green')
