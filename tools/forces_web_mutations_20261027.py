#!/usr/bin/env python3
from pathlib import Path
import subprocess,sys,json
r=Path(sys.argv[1]).resolve();out=Path(sys.argv[2]).resolve();out.mkdir(exist_ok=True,parents=True)
cases=[
 ('ui/misc.js','walls and the Bronze Guard often decide','walls and elite agema often decide','ui/forces_guard.test.mjs','retired Forces labels'),
 ('ui/actornames.js',"elite_infantry: 'Bronze Guard'","elite_infantry: 'Elite Infantry'",'ui/forces_guard.test.mjs','retired Forces labels'),
 ('ui/forces.js','...(people.length?','...(!people.length?','ui/forces.test.mjs','Forces groups by kind'),
 ('ui/movements.js','!u.bound_route_id','true','ui/forces.test.mjs','Marches excludes every bound trade ship'),
 ('ui/drawers/war.js',"if (u.bound_route_name) loc =", "if (false && u.bound_route_name) loc =",'ui/war_cards.test.mjs','route-bound Ships name the route'),
 ('ui/drawers/economy.js','t.standing_order_id === o.id','t.id === o.id','ui/drawers/economy.test.mjs','route cargo uses saved route identity'),
]
proof=[]
for i,(file,old,new,testfile,name) in enumerate(cases):
 p=r/'web/static/js/megaron'/file;before=p.read_bytes();assert old in before.decode(),file
 try:
  p.write_text(before.decode().replace(old,new,1));run=subprocess.run(['node','--test','web/static/js/megaron/'+testfile],cwd=r,text=True,capture_output=True);log=run.stdout+run.stderr;(out/f'{i}-red.log').write_text(log)
  assert run.returncode and 'not ok' in log and name in log and 'SyntaxError' not in log,(file,log)
  proof.append({'file':file,'old':old,'new':new,'test':name,'assertion_red':True})
 finally:p.write_bytes(before)
 assert p.read_bytes()==before
 print('RED + exact restore',file,flush=True)
run=subprocess.run(['node','--test',*[str(p.relative_to(r)) for p in(r/'web').rglob('*.test.mjs')]],cwd=r,text=True,capture_output=True);(out/'restored-green.log').write_text(run.stdout+run.stderr);assert run.returncode==0,run.stdout[-2500:]+run.stderr
(out/'proof.json').write_text(json.dumps({'mutants':proof,'exact_restore':True,'restored_full_js_green':True},indent=2));print('PASS web mutants + full restored JS',flush=True)
