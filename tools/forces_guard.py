#!/usr/bin/env python3
"""Player labels only: preserve wire identifiers, command aliases and source comments."""
from pathlib import Path
import re,json,sys
from day_guard import ROOT,js_strings
old=re.compile(r'\b(?:elite infantry|war chariots?)\b',re.I)
units=re.compile(r'(?<![\w-])units(?![\w-])',re.I)
problems=[];count=0
index=json.loads((ROOT/'web/static/codex/index.json').read_text());titles={a['id']:a['title'] for a in index['articles']}
def check(path,line,text):
 global count
 count+=1
 if old.search(text) or units.search(text):problems.append(f'{path.relative_to(ROOT)}:{line}: {text[:180]}')
for path in (ROOT/'web/static/js/megaron').rglob('*.js'):
 for line,value in js_strings(path.read_text()):
  if re.search(r'/api/|/static/|^\.?\.?/',value):continue
  # A standalone lower-case key is a wire/DOM identifier, not display prose.
  if not re.search(r'\s|[<>]',value) and value not in ['Units']:continue
  value=re.sub(r'<[^>]*>',' ',value)
  check(path,line,value)
for path in (ROOT/'web/static/codex').glob('*.md'):
 text=path.read_text();text=re.sub(r'```[\s\S]*?```|`[^`]*`','',text)
 text=re.sub(r'\[\[([^\]|]+)(?:\|([^\]]+))?\]\]',lambda m:m[2] or titles.get(m[1],m[1]),text)
 for line,value in enumerate(text.splitlines(),1):check(path,line,value)
for entry in index['articles']:check(ROOT/'web/static/codex/index.json',1,entry['title'])
# Game templates and public static mockups: text, not inline code/styles/IDs.
for path in [*(ROOT/'web/static').glob('*.html'),*(ROOT/'web/templates').glob('*.html')]:
 text=re.sub(r'<(?:script|style)\b[^>]*>[\s\S]*?</(?:script|style)>','',path.read_text(),flags=re.I)
 text=re.sub(r'<[^>]*>',' ',text)
 for line,value in enumerate(text.splitlines(),1):check(path,line,value)
if problems:print('\n'.join(problems));raise SystemExit(1)
print(f'Forces player labels: PASS ({count} literal/text rows)')
