#!/usr/bin/env python3
"""Read-only startup evidence; never reads credentials or changes the game."""
import json,pathlib,re
runtime=pathlib.Path('/home/tk/Projects/isladan/playtest')
archive=pathlib.Path('/home/tk/megaron-arkiv/femspelare40-20261020')
result={}
for name in ('hephaistos','talthybios','pheidon'):
    text=(runtime/(name+'.log')).read_text()
    plans=[]
    for match in re.finditer(r'Plan \[(ACT|WAIT|ESCALATE)\].*?(?=\n[^\n]*Turn \d+ —|\Z)',text,re.S):
        block=match.group()
        plans.append({'status':match[1],'ok':block.count('→ ok'),'cli_errors':block.count('CLI error')})
    pid=int((runtime/(name+'.pid')).read_text())
    result[name]={'pid':pid,'running':pathlib.Path(f'/proc/{pid}').exists(),'model_plans':plans,'valid_act_turns':sum(p['status']=='ACT' and p['ok']>0 for p in plans),'auth_errors':len(re.findall(r'HTTP 401|HTTP 403',text)),'parse_errors':len(re.findall(r'Could not parse|Bad JSON|non-object action|No JSON in',text)),'cli_errors':text.count('CLI error')}
model_log=(archive/'llama-35b.log').read_text()
result['llama']={'prompt_tokens':[int(x) for x in re.findall(r'task.n_tokens = (\d+)',model_log)],'truncated_nonzero':len(re.findall(r'truncated = [1-9]',model_log)),'context_shift':len(re.findall(r'context shift|input.*too long|exceeds.*context',model_log,re.I))}
print(json.dumps(result,indent=2))
