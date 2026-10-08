#!/usr/bin/env python3
"""N: protected surfaces and unchanged request/crewing/action blocks."""
import json
from pathlib import Path
import subprocess
ROOT=Path(__file__).resolve().parents[1]
BASE='ba9e6faf'
def git(*args):return subprocess.check_output(['git','-C',str(ROOT),*args],text=True)
path='web/static/js/megaron/ui/drawers/economy.js'
old=git('show',BASE+':'+path);new=(ROOT/path).read_text()
proof={'base':BASE,'head':git('rev-parse','HEAD').strip(),'protected':{}}
for target in ['server','web/static/js/megaron/api.js','web/static/megaron.css','web/static/js/megaron/ui/drawers/diplomacy.js','web/static/js/megaron/ui/drawers/gossip.js']:
 assert not git('diff',BASE,'--',target),target
 proof['protected'][target]='identical'
def block(source,start,end):
 a=source.index(start);b=source.index(end,a)
 return source[a:b]
for name,start,end in [
 ('crewed_by_markup','        <label>Crewed by (which end supplies the gubbe)','        <div>Keep at destination'),
 ('standing_request_block','  const crewedBy =','  const d = await r.json()'),
 ('transfer_request_block','  const r = await fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/provinces/${from}/trade`','  const d = await r.json()'),
 ('route_actions','export async function pauseStandingOrder','async function loadEconomyWants'),
 ('route_render','export function renderStandingOrdersHTML','async function refreshStandingOrders'),
]:
 if name=='crewed_by_markup':
  a='        <label>Crewed by (which end supplies the gubbe)';b='        </label>'
  def crew(src):return block(src,a,b)+b
  assert crew(old)==crew(new),name
 else:assert block(old,start,end)==block(new,start,end),name
 proof[name]='byte-identical'
print(json.dumps(proof,indent=2))
