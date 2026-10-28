from pathlib import Path
import subprocess,json
w=Path('/tmp/megaron-hex-units-mutants');out=Path('/tmp/megaron-codex-hex-units-20261028/docs/reviews/hex-units/mutants');out.mkdir(exist_ok=True)
cases=[('origin','web/static/js/megaron/render/map.js','const at=trade?transportPixelNow(trade):unitHexNow(u);','const at={q:u.q,r:u.r};','js'),('private-hull','web/static/js/megaron/ui/force_card.js','Hull: unknown · Provisions: unknown','Hull: '+"' + u.hull + '"+' · Provisions: unknown','js'),('hull','web/static/js/megaron/ui/force_card.js','(hex || u.hull < 5)','u.hull < 5','js'),('goods','web/static/js/megaron/ui/force_card.js','esc(i.good_key)','esc(i.good)','js'),('pending','web/static/js/megaron/ui/force_card.js','(u.orders_en_route || [])','([])','js'),('cli-origin','server/cmd/keryx/cmd_hex.go','u.Status == "marching" && u.CurrentQ != nil','false && u.CurrentQ != nil','go')]
proof=[]
for name,file,old,new,kind in cases:
 p=w/file;base=p.read_text();assert old in base,(name,old);p.write_text(base.replace(old,new,1))
 cmd=['node','--test','web/static/js/megaron/ui/hex_forces.test.mjs'] if kind=='js' else ['./tools/gotest.sh','./cmd/keryx','-run','TestHexInspectsPhysicalOwnAndOnlyPublicForeignFields']
 try:
  with (out/(name+'-red.log')).open('w') as f:r=subprocess.run(cmd,cwd=w,stdout=f,stderr=subprocess.STDOUT)
  assert r.returncode!=0,name+' survived'
  log=(out/(name+'-red.log')).read_text();assert 'build failed' not in log and 'SyntaxError' not in log,name+' invalid mutant'
 finally:p.write_text(base)
 with (out/(name+'-restored.log')).open('w') as f:r=subprocess.run(cmd,cwd=w,stdout=f,stderr=subprocess.STDOUT)
 assert r.returncode==0,name+' restore failed'
 proof.append({'mutant':name,'red':True,'restored_green':True});print(name,'red -> green',flush=True)
(out/'proof.json').write_text(json.dumps(proof,indent=2))
