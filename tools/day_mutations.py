#!/usr/bin/env python3
"""Physical production-file mutations: eight named guard reds, restored greens."""
from pathlib import Path
import os,subprocess
ROOT=Path(__file__).resolve().parents[1]
OUT=ROOT/'docs/reviews/day/mutations';OUT.mkdir(parents=True,exist_ok=True)
def cleanlog(value):return '\n'.join(line.rstrip() for line in value.splitlines())+'\n'
arms=[
 ('web', 'web/static/js/megaron/ui/fmt_num.js', "' day'", "' game day'"),
 ('keryx','server/cmd/keryx/output.go','qualifier + "day"','qualifier + "game day"'),
 ('codex','web/static/codex/time.md','one **day** at a time','one **game day** at a time'),
 ('server','server/internal/tick/eta.go','"1 day"','"1 game day"'),
]
# Web singular literal is emitted with the ternary form after rounding.
arms[0]=('web',arms[0][1],"? 'day' : 'days'", "? 'game day' : 'days'")
for surface,file,needle,mutation in arms:
 for kind in ['unit','wall']:
  path=ROOT/file;original=path.read_text();assert original.count(needle)==1,(surface,needle)
  changed=mutation if kind=='unit' else ('"≈ 3 days real time"' if path.suffix=='.go' else "'≈ 3 days real time'" if path.suffix=='.js' else '≈ 3 days real time')
  # Replace the whole prose token for the wall mutation, maintaining valid source.
  if kind=='wall':
   if surface=='web': mutated=original.replace("'days'",changed,1)
   elif surface=='keryx':mutated=original.replace('"days"',changed,1)
   elif surface=='server':mutated=original.replace('"1 day"',changed,1)
   else:mutated=original.replace(needle,changed,1)
  else:mutated=original.replace(needle,mutation,1)
  label=surface+'-'+kind
  try:
   path.write_text(mutated)
   if surface in ('web','codex'):command=['node','--test','web/static/js/megaron/ui/day_guard.test.mjs']
   else:command=['go','test','./'+('cmd/keryx' if surface=='keryx' else 'internal/tick'),'-run','TestDayPlayerLanguageGuard','-count=1']
   cwd=ROOT/'server' if surface in ('keryx','server') else ROOT
   result=subprocess.run(command,cwd=cwd,env={k:os.environ[k] for k in ('HOME','PATH')},text=True,capture_output=True)
   log=result.stdout+result.stderr;(OUT/(label+'-red.log')).write_text(cleanlog(log))
   assert result.returncode and ('Day guard '+surface) in log and 'forbidden player time' in log,(label,log)
  finally:path.write_text(original)
  result=subprocess.run(command,cwd=cwd,env={k:os.environ[k] for k in ('HOME','PATH')},text=True,capture_output=True)
  (OUT/(label+'-restored.log')).write_text(cleanlog(result.stdout+result.stderr))
  assert result.returncode==0,(label,result.stdout+result.stderr)
  print(label+': named guard red → restored green',flush=True)
# Nested prose and duplicate formerly-exempt identifiers must also be visible.
for label,surface,file,extra in [
 ('web-nested','web','web/static/js/megaron/ui/fmt_num.js',"\nconst dayGuardMutation = `${true ? 'game day' : 'day'}`;\n"),
 ('keryx-extra-key','keryx','server/cmd/keryx/cmd_unit.go','\nconst dayGuardMutation = "ticks"\n'),
]:
 path=ROOT/file;original=path.read_text()
 command=['node','--test','web/static/js/megaron/ui/day_guard.test.mjs'] if surface=='web' else ['go','test','./cmd/keryx','-run','TestDayPlayerLanguageGuard','-count=1']
 cwd=ROOT if surface=='web' else ROOT/'server'
 try:
  path.write_text(original+extra)
  result=subprocess.run(command,cwd=cwd,capture_output=True,text=True)
  log=result.stdout+result.stderr;(OUT/(label+'-red.log')).write_text(cleanlog(log))
  assert result.returncode and 'Day guard '+surface in log and 'forbidden player time' in log,(label,log)
 finally:path.write_text(original)
 result=subprocess.run(command,cwd=cwd,capture_output=True,text=True)
 (OUT/(label+'-restored.log')).write_text(cleanlog(result.stdout+result.stderr))
 assert result.returncode==0,(label,result.stdout+result.stderr)
 print(label+': named guard red → restored green',flush=True)
