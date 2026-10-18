#!/usr/bin/env python3
"""Extract executed literal CLI inputs/outputs from one local player turn only.

Never exports reasoning, prompts, auth config or unrelated session history.
Usage: collect_round_tools.py SESSION.jsonl TURN_ID OUT.json
Supports the double-quoted command literals used by these player agents.
"""
import json,re,sys
from pathlib import Path
rows=[json.loads(line) for line in Path(sys.argv[1]).open()]
turn=sys.argv[2];start=None;end=len(rows)
for i,row in enumerate(rows):
 p=row.get('payload',{})
 if row.get('type')=='event_msg' and p.get('type')=='task_started':
  if start is not None:end=i;break
  if p.get('turn_id')==turn:start=i
if start is None:raise SystemExit('turn not found')
calls=[]
for row in rows[start:end]:
 p=row.get('payload',{});kind=p.get('type');cid=p.get('call_id')
 if row.get('type')!='response_item':continue
 if kind in ('custom_tool_call','function_call'):
  source=p.get('input',p.get('arguments',''));commands=[]
  for match in re.finditer(r'"(?:\\.|[^"\\])*"',source):
   try:value=json.loads(match.group())
   except ValueError:continue
   if '/home/tk/go/bin/poleia' in value and 'POLEIA_CONFIG=' in value:commands.append(value)
  if '/home/tk/go/bin/poleia' in source and not commands:raise SystemExit('unsupported dynamic command; inspect manually')
  calls.append({'call_id':cid,'commands':commands,'output':[]})
 elif kind in ('custom_tool_call_output','function_call_output'):
  for call in calls:
   if call['call_id']==cid:call['output'].append(p.get('output'))
calls=[call for call in calls if call['commands']]
result={'turn':turn,'commands_count':sum(len(c['commands']) for c in calls),'calls':calls}
text=json.dumps(result,ensure_ascii=False,indent=2)+'\n'
if 'eyJhbGciOi' in text:raise SystemExit('refusing token-like output')
Path(sys.argv[3]).write_text(text)
print(result['commands_count'],'executed CLI commands')
