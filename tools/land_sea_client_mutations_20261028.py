#!/usr/bin/env python3
from pathlib import Path
import subprocess,sys,json
root=Path(sys.argv[1]).resolve();out=Path(sys.argv[2]).resolve();out.mkdir(parents=True,exist_ok=True)
cases=[
 ('server/cmd/keryx/cmd_gossip.go','"by":           by,','"by":           "sea",',['go','-C','server','test','./cmd/keryx','-run','TestRouteAndTradeOffer','-count=1'],'TestRouteAndTradeOffer'),
 ('web/static/js/megaron/ui/transport_choice.js',"o.available?'':' disabled'","true?'':' disabled'",['node','--test','web/static/js/megaron/ui/transport_choice.test.mjs'],'transport choices show both routes'),
 ('web/static/js/megaron/ui/transport_choice.js','[preferred,data.selected_by,','[data.selected_by,preferred,',['node','--test','web/static/js/megaron/ui/transport_choice.test.mjs'],'transport choices preserve explicit'),
 ('web/static/js/megaron/ui/drawers/economy.js',"{by:selectedTransportBy('ec-tr-route')}","{by:'land'}",['node','--test','web/static/js/megaron/ui/drawers/economy_rows.test.mjs'],'transfer sends selected sea'),
 ('web/static/js/megaron/ui/drawers/economy.js',"{by:selectedTransportBy('ec-so-route')}","{by:'sea'}",['node','--test','web/static/js/megaron/ui/drawers/economy_rows.test.mjs'],'land/sea choice reaches standing'),
 ('web/static/js/megaron/ui/drawers/diplomacy.js','body.trade_offer.by=by',"body.trade_offer.by='land'",['node','--test','web/static/js/megaron/ui/drawers/diplomacy_simple.test.mjs'],'trade offer sends selected goods mode'),
 ('server/cmd/keryx/cmd_goods.go','payload["by"] = by','payload["by"] = "land"',['go','-C','server','test','./cmd/keryx','-run','TestTransferCarriesExplicit','-count=1'],'TestTransferCarriesExplicit'),
 ('server/cmd/keryx/cmd_route.go','"by":                      by,','"by":                      "sea",',['go','-C','server','test','./cmd/keryx','-run','TestRouteAndTradeOffer','-count=1'],'TestRouteAndTradeOffer'),
 ('server/cmd/keryx/cmd_messenger_split.go','tradeOffer["by"] = by','tradeOffer["by"] = "sea"',['go','-C','server','test','./cmd/keryx','-run','TestRouteAndTradeOffer','-count=1'],'TestRouteAndTradeOffer'),
 ('server/cmd/keryx/transport_choice.go','by != "" && by != "land" && by != "sea"','by != "" && by != "land"',['go','-C','server','test','./cmd/keryx','-run','TestTransferCarriesExplicit','-count=1'],'TestTransferCarriesExplicit'),
]
proof=[]
for i,(file,old,new,cmd,name) in enumerate(cases):
 p=root/file;before=p.read_bytes();assert old in before.decode(),(file,old)
 try:
  p.write_text(before.decode().replace(old,new,1));r=subprocess.run(cmd,cwd=root,text=True,capture_output=True);log=r.stdout+r.stderr;(out/f'{i}-red.log').write_text(log)
  assert r.returncode and ('not ok' in log or '--- FAIL:' in log) and name in log and 'build failed' not in log and 'SyntaxError' not in log,(file,log)
  proof.append({'file':file,'old':old,'new':new,'command':cmd,'assertion':name,'red':True})
 finally:p.write_bytes(before)
 assert before==p.read_bytes();print('RED + exact restore',file,flush=True)
for name,cmd in [('js',['node','--test',*[str(p.relative_to(root)) for p in(root/'web').rglob('*.test.mjs')]]),('cli',['go','-C','server','test','./cmd/keryx','-count=1'])]:
 r=subprocess.run(cmd,cwd=root,text=True,capture_output=True);(out/f'restored-{name}-green.log').write_text(r.stdout+r.stderr);assert r.returncode==0,(name,r.stdout[-2000:],r.stderr)
(out/'proof.json').write_text(json.dumps({'mutants':proof,'exact_restore':True,'restored_full_js_and_cli_green':True},indent=2));print('PASS client mutations and restored full JS/CLI',flush=True)
