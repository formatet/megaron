#!/usr/bin/env python3
"""Apply the player-unit wording to guard-selected literals; preserve contract keys.
Used for this language migration; tests use the same spelling in expectations.
"""
from pathlib import Path
import importlib.util,re
ROOT=Path(__file__).resolve().parents[1]
spec=importlib.util.spec_from_file_location('guard',ROOT/'tools/day_guard.py');guard=importlib.util.module_from_spec(spec);spec.loader.exec_module(guard)
def wording(value):
 protected=[]
 def keep(m):protected.append(m[0]);return f'@@CALENDAR{len(protected)-1}@@'
 value=re.sub(r'\bShadow Days of the Goddess\b|\bDay\s+\d+\s+of\b[^.\n]*',keep,value)
 value=re.sub(r'(?<!game )(?<!game-)\bdays?\b',lambda m:'game '+m[0].lower(),value,flags=re.I)
 for i,v in enumerate(protected):value=value.replace(f'@@CALENDAR{i}@@',v)
 return value
if __name__=='__main__':
 for surface in ('web','keryx','codex','server'):
  for p in guard.sources(surface):
   source=p.read_text();s=source
   for _,value in guard.literals(p):
    if not guard.bare_days(value):continue
    if str(p.relative_to(ROOT))=='server/internal/gossip/gossip.go':continue
    new=wording(value)
    if value in ('day','days'):
     s=re.sub(r'([\'"`])'+re.escape(value)+r'\1' if p.suffix=='.js' else r'\1(?!\s*:)' ,lambda m:m[1]+new+m[1],s)
    else:s=s.replace(value,new)
   if s!=source:p.write_text(s)
