#!/usr/bin/env python3
"""Frozen snapshot projection; gift order, per-temple staffing and scarcity from owners."""
from pathlib import Path
import json
out=Path(__file__).resolve().parent;s=json.loads((out/'live-snapshot.json').read_text());v=s['values'];base=s['base_values'];grain=v.get('grain',0);ref=base.get('grain',0)
scarcity=max(1,min(2,grain/ref)) if grain>0 and ref>0 else 1
rows=[]
for w in sorted(s['wanaxes'],key=lambda w:w['username']):
 cities=[c for c in s['cities'] if c['owner_id']==w['player_id']];pop=sum(c['population'] for c in cities);workers=fed=0
 for c in cities:
  stock=c['stock'].copy()
  for t in c['temples']:
   workers+=t['staffed'];gift={};remaining=.25*grain
   if stock.get('grain',0)>=.25:gift={'grain':.25};paid=True
   elif remaining<=0:paid=False
   else:
    paid=False
    for good in ['grain','fish','timber','silver']:
     have=stock.get(good,0);value=v.get(good,0)
     if have<=0 or value<=0:continue
     need=remaining/value
     if have>=need:gift[good]=need;paid=True;break
     gift[good]=have;remaining-=have*value
   if paid:
    fed+=t['staffed']
    for good,amount in gift.items():stock[good]-=amount
 n=len(cities);old=.1+.1*n*(n-1)/2;new=.1+.2*max(0,n-1)+.1*(pop/1000);gain=fed*1.05*scarcity
 rows.append(dict(username=w['username'],player_id=w['player_id'],cities=n,population=pop,workers=workers,fed_workers=fed,scarcity=scarcity,gain=gain,old_burden=old,new_burden=new,old_net=gain-old,new_net=gain-new))
(out/'simulation.json').write_text(json.dumps(rows,indent=2)+'\n')
header='| Wanax | Städer | Invånare | Matade/placerade tempelgubbar | Produktion | Belastning före | Efter | Netto före | Efter |'
lines=[header,'|'+'---|'*9]
for r in rows:lines.append('| '+ ' | '.join([r['username'],str(r['cities']),str(r['population']),f"{r['fed_workers']}/{r['workers']}",*[f"{r[k]:.4f}" for k in ['gain','old_burden','new_burden','old_net','new_net']]])+' |')
negative=[r['username'] for r in rows if r['new_net']<0];founded=[r for r in rows if r['cities']>0];negf=[r['username'] for r in founded if r['new_net']<0]
text='''# ORDER24 — kultförsörjning efter städer och invånare

## Steg 0: skrivskyddad simulering före kod

'''+f"Snapshot {s['observed_at']}, tick{s['tick']}, BEGIN READ ONLY={s['transaction_read_only']}, mig{s['migration']['version']}. Körande healthz: {(out/'healthz.json').read_text().strip()}.\n\n"+'\n'.join(lines)+f'''\n\nNegativt efter: {len(negative)}/{len(rows)} samtliga PWR ({', '.join(negative)}); bland stadsägare {len(negf)}/{len(founded)} ({', '.join(negf)}). Nytt negativt: {', '.join(r['username'] for r in rows if r['old_net']>=0 and r['new_net']<0) or 'ingen'}.

Produktion är planerad nästa ticks brutto från just de tempelgubbar vars eget offer kan betalas, inte nominell kapacitet. Knapphetsfaktor {scarcity:.8f}, härledd från snapshotens gudavärde för korn / goods.labor_value och klämd till1–2. Lokala lager settled() utan påhittad capklämning, tempel i gud-ID-ordning,2platser/nivå. Inga andra framtida förändringar antas. Nettot är före golv/tak. PWR utan städer visas separat från stadsägare; ingen negativ extra-stadterm för0städer. Stadsfria raden visar formel/projektionsnetto, inte faktisk Hostdebitering: KharisTick väljer kapitalägare.

Invånarkälla: settlements.population i samma icke-avslutade städer som stadsräkning och tempelläsare (state NOT IN sunk/collapsed/razed); detta är stadsbefolkningen som mat/tillväxt läser. Inte units.size, gubbar avrundade till hundratal eller lagrad födelseprognos.

Ingen kod eller DBskrivning föregick denna simulering. capture.py/snapshot.sql och simulate.py gör underlaget reproducerbart.
'''
previous=(out/'README.md').read_text() if (out/'README.md').exists() else ''
marker='## Kontrakt och implementation'
if marker in previous:text+='\n'+marker+previous.split(marker,1)[1]
(out/'README.md').write_text(text);print('\n'.join(lines));print(f'negative {len(negative)}/{len(rows)}; founded {len(negf)}/{len(founded)}')
