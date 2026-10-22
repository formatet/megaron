#!/usr/bin/env python3
"""Compare saved payroll facts; never mutates or contacts the live database."""
from pathlib import Path
import json
p=Path(__file__).resolve().parent
s=json.loads((p/'snapshot.json').read_text())
raw=(p/'runtime-share.txt').read_text()
share=.7
for line in raw.splitlines():
    if line.startswith('UPKEEP_SOLD_SHARE='):
        try: share=max(0,min(1,float(line.split('=',1)[1])))
        except ValueError:pass
old={'spearman':0,'elite_infantry':2,'war_chariot':3,'galley':1.5,'war_galley':2.5,'merchantman':1,'nomadic_host':0}
factors={'spearman':0,'elite_infantry':1.5,'war_chariot':2,'galley':1,'war_galley':1,'merchantman':1,'nomadic_host':0}
home={'garrison','repairing','fitting_out'}
field={'marching','positioned','freighting','embarked'}
rows=[]
for u in s['units']:
    if u['type'] not in old:raise SystemExit('unclassified type '+u['type'])
    active=u['status'] in home|field and not u['subsistence']
    gross=old[u['type']]*(u['size']/100 if u['category']=='land' else 1) if active else 0
    atpayer=bool(u['valid_payer_id']) and u['settlement_id']==u['valid_payer_id']
    credit=gross*share if atpayer else 0
    new=factors[u['type']]*(max(0,u['crew'])/100 if u['category']=='naval' else u['size']/100)*(.5 if u['status'] in home else 1) if active else 0
    row={**u,'payroll_active':active,'old_gross':gross,'old_credit':credit,'old_net_if_paid':gross-credit,'new_net_if_paid':new,'delta':new-(gross-credit)}
    rows.append(row)
(p/'comparison.json').write_text(json.dumps(rows,indent=2,ensure_ascii=False)+'\n')
lines=['# Snapshot payroll comparison','',f"Observed {s['observed_at']}; migration{s['migration']}; transaction_read_only={s['transaction_read_only']}; share={share}.",'', '| Unit | Type | Status | Size/Crew | Payer | Old gross | Old credit | Old net | New net | Delta |','|---|---|---|---|---|---:|---:|---:|---:|---:|']
for u in rows:
    lines.append('| '+' | '.join([u['id'],u['type'],u['status'],str(u['size'])+'/'+str(u['crew']),str(u['payer_name'])]+[f'{u[k]:.6g}' for k in ['old_gross','old_credit','old_net_if_paid','new_net_if_paid','delta']])+' |')
active=[u for u in rows if u['world_status']=='active']
lines+=['',f"All active-world units ({len(active)}): old net {sum(u['old_net_if_paid'] for u in active):.6g}, new {sum(u['new_net_if_paid'] for u in active):.6g} silver/tick.",'','Only operational, non-subsistence units are charged. Forming/laid_up are not current pay. These are scheduled payroll needs if paid, not proof of actual affordability or a historical deduction. No treasuries pooled; payer ownership stays unchanged.']
(p/'comparison.md').write_text('\n'.join(lines)+'\n')
print('\n'.join(lines[-4:]))
