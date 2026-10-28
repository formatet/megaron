#!/usr/bin/env python3
"""Physical storm refresh mutants in an isolated checkout; exact restore always."""
from pathlib import Path
import sys,subprocess,json,shutil
R=Path(sys.argv[1]).resolve();O=Path(__file__).resolve().parent/'mutants';O.mkdir(exist_ok=True)
rows=[]
def mutate(name,file,old,new,cmd,needle):
 p=R/file;before=p.read_bytes();assert old in before.decode(),name
 try:
  p.write_text(before.decode().replace(old,new,1));r=subprocess.run(cmd,cwd=R,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT);(O/(name+'.log')).write_text(r.stdout);assert r.returncode and needle in r.stdout,(name,r.returncode,r.stdout[-1000:]);rows.append({'name':name,'assertion':needle,'exit':r.returncode})
 finally:p.write_bytes(before)
 assert p.read_bytes()==before
mutate('refresh-disabled','web/static/js/megaron/ui/stormname.js',"for(const el of root?.querySelectorAll?.('[data-storm-q][data-storm-r]')||[])","for(const el of [])",['node','--test','web/static/js/megaron/ui/stormname.test.mjs'],'worked water storm slot follows')
# Run the final acceptance harness with only the periodic poll reverted.
shutil.copy(Path(__file__).resolve().parents[3]/'tools/storm_hex_repro_20261028.py',R/'tools/storm_hex_repro_20261028.py')
mutate('poll-bypasses-refresh','web/static/js/megaron/render/map.js','updateStormData(d.storms || []);','State.stormData = d.storms || []; State.dirty = true;',['python3','tools/storm_hex_repro_20261028.py',str(O/'poll-native'),'--expect-live'],'AssertionError')
r=subprocess.run(['node','--test','web/static/js/megaron/ui/stormname.test.mjs','web/static/js/megaron/ui/citygrid.test.mjs'],cwd=R,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT);(O/'restored-green.log').write_text(r.stdout);assert r.returncode==0,r.stdout
(O/'summary.json').write_text(json.dumps({'base':'2a0ca7fd','physical_mutants':rows,'production_exact_restore':True,'restored_js_exit':0},indent=2)+'\n');print('PASS two physical mutants, exact restored source and green JS')
