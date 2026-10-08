#!/usr/bin/env python3
"""Verify M leaves existing trade/reply consumers and protected surfaces intact."""
from pathlib import Path
import hashlib,json,subprocess
ROOT=Path(__file__).resolve().parents[1]
base='6d8d5f60';path='web/static/js/megaron/ui/drawers/diplomacy.js'
before=subprocess.check_output(['git','-C',str(ROOT),'show',base+':'+path],text=True);after=(ROOT/path).read_text();evidence={}
for name in ['dipToggleKind','dipSendInThread','dipCancel','dipAccept','dipDecline','dipReply','dipArrangePassage','dipCallBack','tradeScheduleText']:
 def extract(text):
  start=text.index('export '+('async ' if 'export async function '+name in text else '')+'function '+name)
  brace=text.index('{',start);depth=0
  for i in range(brace,len(text)):
   if text[i]=='{':depth+=1
   if text[i]=='}':
    depth-=1
    if depth==0:return text[start:i+1]
 a,b=extract(before),extract(after);assert a==b,name;evidence[name]=hashlib.sha256(a.encode()).hexdigest()
a=before[before.index('      // Inline compose'):before.index("      html += '</div></div>'")];b=after[after.index('      // Inline compose'):after.index("      html += '</div></div>'")];assert a==b;evidence['inline-trade-composer']=hashlib.sha256(a.encode()).hexdigest()
protected=['server','web/static/js/megaron/ui/drawers/gossip.js','web/static/js/megaron/render/map.js','web/static/megaron.css']
assert not subprocess.check_output(['git','-C',str(ROOT),'diff',base,'--',*protected]),'protected surface changed'
(ROOT/'docs/reviews/forenkling-diplomacy/logs/unchanged-trade-consumers.json').write_text(json.dumps(evidence,indent=2)+'\n')
print('Existing trade/dispatch/reply consumers, inline form and protected surfaces unchanged')
