from pathlib import Path
import subprocess,json
w=Path('/tmp/megaron-river-flow-mutants'); out=Path('/tmp/megaron-codex-river-flow-20261028/docs/reviews/river-flow/mutants');out.mkdir(exist_ok=True)
subprocess.run(['git','-C',str(w),'reset','--hard','78a4be43'],check=True,capture_output=True)
cases=[('saved-values','server/internal/world/river_metadata_store.go','AND q=$2 AND r=$3 AND river_edges IS NULL','AND q=$2 AND r=$3','go',['./internal/world','-run','TestRiverMetadataBackfillIsAtomicAndPreservesSavedValues']),('delta-mouth','server/internal/world/river_flow.go','degree[c] <= 1 && delta && open','degree[c] <= 1 && open','go',['./internal/world','-run','TestRiverFlowUsesSeaDeltaMouthNotSourceWater']),('writer','server/internal/world/river_metadata.go','if !t.RiverMetadataReady {','if true {','go',['./api/handlers','-run','TestStoreTilesPersistsGeneratedRiverGeography']),('fog','server/api/handlers/world.go','if !tiles[i].Visible {','if false {','go',['./api/handlers','-run','TestMapRiver']),('edge','web/static/js/megaron/render/markraster.js','if(declared)return','if(false && declared)return','js',['web/static/js/megaron/render/map.test.mjs']),('ink','web/static/js/megaron/render/markraster.js',"if (eq === q && er === r && t === 'fog' && hasRiverEdges(q,r) && riverEdges(q,r).length) return true;",'','js',['web/static/js/megaron/render/map.test.mjs']),('direction','web/static/js/megaron/ui/river_flow.js','nx/=len;ny/=len;','nx/=-len;ny/=-len;','js',['web/static/js/megaron/ui/river_flow.test.mjs'])]
results=[]
for name,file,old,new,kind,args in cases:
 p=w/file;base=p.read_text();assert old in base
 changed=base.replace(old,new,1)
 if name=='delta-mouth':changed=changed.replace('delta, open := false, false','_, open := false, false').replace('delta = true','_ = true')
 if name=='fog':changed=changed.replace('t.savedRiverEdges != nil && (t.Terrain == "river" || t.Terrain == "river_ford")','t.savedRiverEdges != nil')
 p.write_text(changed)
 cmd=['./tools/gotest.sh',*args] if kind=='go' else ['node','--test',*args]
 with (out/(name+'-red.log')).open('w') as f:r=subprocess.run(cmd,cwd=w,stdout=f,stderr=subprocess.STDOUT)
 p.write_text(base)
 assert r.returncode!=0,name+' survived'
 log=(out/(name+'-red.log')).read_text();assert 'build failed' not in log and 'syntax error' not in log,name+' invalid mutant'
 with (out/(name+'-restored.log')).open('w') as f:g=subprocess.run(cmd,cwd=w,stdout=f,stderr=subprocess.STDOUT)
 assert g.returncode==0,name+' restore failed'
 results.append({'mutant':name,'red':r.returncode,'restored':g.returncode});print(name,'red -> green',flush=True)
(out/'proof.json').write_text(json.dumps(results,indent=2))
