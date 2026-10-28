#!/usr/bin/env python3
"""One-time historical fact conservation audit; not a permanent freeze of article prose."""
from pathlib import Path
import re,json,subprocess,hashlib
R=Path(__file__).resolve().parents[1];D=R/'web/static/codex';OUT=R/'docs/reviews/wiki-catalog';baseline=json.loads((OUT/'fact-inventory.json').read_text());moves=json.loads((OUT/'fact-migration.json').read_text())
def norm(s):
 s=re.sub(r'<!--[\s\S]*?-->','',s);s=re.sub(r'\[\[([^]|]+)\|([^]]+)\]\]',r'\2',s);s=re.sub(r'\[\[([^]]+)\]\]',r'\1',s);s=s.replace('’',"'").replace('‘',"'")
 return ' '.join(re.sub(r'[`*#|]',' ',s).lower().split())
def atoms(s):
 if len(s.split())>170 and re.match(r'^(?:- |\d+\. )',s):return [x.strip() for x in re.split(r'\n(?=(?:- |\d+\. ))',s) if x.strip()]
 if len(s.split())>170 and s.startswith('|'):
  lines=s.splitlines();return ['\n'.join(lines[:2])+'\n'+row for row in lines[2:] if row.strip()]
 return [s]
bodies={p.stem:norm(p.read_text()) for p in D.glob('*.md')};matched=[];missing=[];corrections=[]
latest_sea=subprocess.check_output(['git','-C',str(R),'show','f45e4cbd:web/static/codex/sea.md'],text=True)
for packet in baseline['blocks']:
 for atom in atoms(packet['source_markdown']):
  target=norm(atom)
  if not target:continue
  correction=None
  if packet['source_article']=='sea' and 'that city still supplies' in atom:
   atom=next(p for p in latest_sea.split('\n\n') if p.startswith('The ship sails to the city'));target=norm(atom);correction='Timothy2026-10-28 physical departure port; deployed d09a74af'
  for move in moves['records']:
   if 'correction' in move and norm(move['text'])==target:
    target=norm(move['correction']['replacement']);correction=move['correction']['reason'];break
  homes=[id for id,text in bodies.items() if target in text]
  record={'old_article':packet['source_article'],'line':packet['source_line'],'source_sha256':hashlib.sha256(atom.encode()).hexdigest(),'homes':homes}
  if correction:record['correction']=correction;corrections.append(record)
  (matched if homes else missing).append(record if homes else {**record,'unmatched':atom})
proof={'baseline':baseline['baseline'],'source_blocks':len(baseline['blocks']),'matched_atoms':len(matched),'missing_atoms':len(missing),'corrections':corrections,'records':matched,'missing':missing}
(OUT/'fact-preservation-proof.json').write_text(json.dumps(proof,ensure_ascii=False,indent=2)+'\n')
print('Historical Codex facts:',len(matched),'matched;',len(missing),'missing;',len(corrections),'source-backed corrections')
if missing:
 for row in missing:print(row['old_article'],row['line'],row['unmatched'][:160])
 raise SystemExit(1)
