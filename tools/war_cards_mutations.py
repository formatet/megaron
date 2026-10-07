#!/usr/bin/env python3
"""Physical I mutations: missing ETA, hidden stance, naval action gate.
Usage: python3 tools/war_cards_mutations.py OUT
"""
from pathlib import Path
import os,subprocess,sys
ROOT=Path(__file__).resolve().parents[1]
OUT=Path(sys.argv[1]);OUT.mkdir(parents=True,exist_ok=True)
base=ROOT/'web/static/js/megaron/ui'
arms=[('arrival',base/'time.js',"export function arrivalHTML(iso, arrivalTick, doneWord = 'arrived') {\n  const ms = msUntil(iso, arrivalTick);\n  if (!Number.isFinite(ms)) return '';", "export function arrivalHTML(iso, arrivalTick, doneWord = 'arrived') {\n  const ms = msUntil(iso, arrivalTick);", 'time.test.mjs','unknown arrival must be empty'),
 ('stance',base/'drawers/war.js',"moreActions += '<select id=\"ustance-", "actions += '<select id=\"ustance-",'war_cards.test.mjs','stance must stay under More'),
 ('naval',base/'drawers/war.js','if (isMarching && !isNaval) {','if (isMarching) {','war_cards.test.mjs','ships must never promise recall or redirect')]
for label,path,needle,mutation,test,assertion in arms:
 original=path.read_text();assert original.count(needle)==1,(label,original.count(needle))
 try:
  path.write_text(original.replace(needle,mutation))
  result=subprocess.run(['node','--test',str(base/test)],cwd=ROOT,env={k:os.environ[k] for k in ('HOME','PATH')},capture_output=True,text=True)
  (OUT/(label+'-red.log')).write_text(result.stdout+result.stderr)
  assert result.returncode and assertion in result.stdout,(label,result.stdout)
 finally:path.write_text(original)
 result=subprocess.run(['node','--test',str(base/test)],cwd=ROOT,env={k:os.environ[k] for k in ('HOME','PATH')},capture_output=True,text=True)
 (OUT/(label+'-restored.log')).write_text(result.stdout+result.stderr)
 assert result.returncode==0,label
 print(label+': named assertion red -> restored green',flush=True)
