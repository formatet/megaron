#!/usr/bin/env python3
"""Mutate actual return-post consumers, require red, restore bytes, require green."""
from pathlib import Path
import hashlib,json,subprocess
ROOT=Path(__file__).resolve().parents[1]
OUT=ROOT/'docs/reviews/return-post';OUT.mkdir(parents=True,exist_ok=True)
results=[]
for name,path,old,new,cmd,cwd in [
 ('queue-reopens-closed','web/static/js/megaron/ui/drawers/diplomacy_queue.js',"const actions = item.reply_window_closed ? supplied.filter(a => a !== 'reply') : supplied;","const actions = supplied;",['node','--experimental-default-type=module','--test','--test-name-pattern','return-post actions','web/static/js/megaron/ui/drawers/diplomacy_queue.test.mjs'],ROOT),
 ('thread-reopens-closed','web/static/js/megaron/ui/drawers/diplomacy.js',"const replyRow = !offer && m.status === 'delivered' && !m.reply_window_closed","const replyRow = !offer && m.status === 'delivered'",['node','--experimental-default-type=module','--test','--test-name-pattern','actual legacy Correspondence hides closed','web/static/js/megaron/ui/drawers/diplomacy_simple.test.mjs'],ROOT),
 ('cli-reopens-closed','server/cmd/keryx/cmd_marches.go','if m["reply_window_closed"] == true {','if false {',['go','test','./cmd/keryx','-run','TestReturnPostOrdinaryInbox','-count=1'],ROOT/'server'),
 ('thread-swallows-cutoff-error','web/static/js/megaron/ui/drawers/diplomacy.js',"    showInlineResult(row, formatApiError(data, 'Reply failed'), true);","    // Mutation: ignore the refusal.",['node','--experimental-default-type=module','--test','--test-name-pattern','legacy return-post refusal','web/static/js/megaron/ui/drawers/diplomacy_simple.test.mjs'],ROOT),
]:
 p=ROOT/path;original=p.read_bytes();assert old.encode() in original
 try:
  p.write_bytes(original.replace(old.encode(),new.encode(),1));r=subprocess.run(cmd,cwd=cwd,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT);(OUT/(name+'-red.log')).write_text(r.stdout);assert r.returncode!=0,name+' survived'
 finally:p.write_bytes(original)
 assert hashlib.sha256(p.read_bytes()).digest()==hashlib.sha256(original).digest()
 r=subprocess.run(cmd,cwd=cwd,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT);(OUT/(name+'-restored.log')).write_text(r.stdout);assert r.returncode==0,r.stdout
 results.append(dict(mutation=name,source=path,sha256=hashlib.sha256(original).hexdigest(),mutant='RED',restored='GREEN'))
(OUT/'client-mutations.json').write_text(json.dumps(results,indent=2));print('Four return-post consumer mutants RED; exact source restored GREEN')
