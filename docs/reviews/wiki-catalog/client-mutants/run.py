from pathlib import Path
import subprocess,json
R=Path('/tmp/megaron-wiki-client-mutants');O=R/'docs/reviews/wiki-catalog/client-mutants';O.mkdir(parents=True,exist_ok=True)
rows=[]
def run(name,path,old,new,args,needle):
 p=R/path;s=p.read_text();assert old in s,(name,old);p.write_text(s.replace(old,new,1))
 try:
  r=subprocess.run(args,cwd=R,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT);(O/(name+'.log')).write_text(r.stdout);assert r.returncode and needle in r.stdout,(name,r.returncode,r.stdout[-1000:]);rows.append({'mutant':name,'exit':r.returncode,'assertion':needle})
 finally:p.write_text(s)
js=['node','--test','web/static/js/megaron/ui/codex.test.mjs']
run('alias-removed','web/static/js/megaron/ui/codex.js',"aliases=(a.aliases||[]).map(normalise)","aliases=[]",js,'Codex search ranks titles')
run('title-priority-reversed','web/static/js/megaron/ui/codex.js','title===needle?0','title===needle?4',js,'Codex search ranks titles')
run('unit-entity-wrong','web/static/js/megaron/ui/drawers/war.js',"esc(u.type)","esc('wrong')",['node','--test','web/static/js/megaron/ui/war_cards.test.mjs'],'Forces name opens')
run('generated-cost-corrupt','web/static/codex/granary.md','12 timber','13 timber',js,'generated Codex facts are current')
run('bundle-corrupt','web/static/codex/articles.json','A Harbour','A WRONG Harbour',js,'article bundle matches')
run('cli-alias-removed','server/cmd/keryx/cmd_codex.go','for _, alias := range a.Aliases {','for _, alias := range []string{} {',['tools/gotest.sh','./cmd/keryx','-run','^TestCodexSearchTitleAliasBodyOrderAndSafeResolution$'],'resolve "mana"')
r=subprocess.run(['node','--test','web/static/js/megaron/ui/codex.test.mjs','web/static/js/megaron/ui/war_cards.test.mjs'],cwd=R,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT);(O/'restore.log').write_text(r.stdout);assert r.returncode==0,r.stdout
r=subprocess.run(['tools/gotest.sh','./cmd/keryx','-run','^TestCodexSearchTitleAliasBodyOrderAndSafeResolution$'],cwd=R,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT);(O/'restore-cli.log').write_text(r.stdout);assert r.returncode==0,r.stdout
(O/'summary.json').write_text(json.dumps({'base':'c87e750d','physical_mutants':rows,'restored_js_and_fresh_cli_green':True},indent=2)+'\n');print('PASS',len(rows),'physical mutants and restored JS/fresh CLI')
