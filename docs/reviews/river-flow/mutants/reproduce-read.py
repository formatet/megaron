from pathlib import Path
import subprocess
w=Path('/tmp/megaron-river-flow-mutants');out=Path('/tmp/megaron-codex-river-flow-20261028/docs/reviews/river-flow/mutants');p=w/'server/api/handlers/world.go';base=p.read_text();old='\t\t\tt.RiverEdges = &edges';new='''			var width, height int
			if err := h.pool.QueryRow(r.Context(), "SELECT map_width,map_height FROM worlds WHERE id=$1", worldID).Scan(&width,&height); err != nil { panic(err) }
			flow := world.RiverFlows(fullMap,width,height)[[2]int{t.Q,t.R}]
			up,down = flow.Upstream,flow.Downstream
'''+old
assert old in base;p.write_text(base.replace(old,new,1));cmd=['./tools/gotest.sh','./api/handlers','-run','TestMapRiverFlowUsesFullSavedGeographyButOnlyExposesKnownTile']
try:
 with (out/'read-recalculates-red.log').open('w') as f:r=subprocess.run(cmd,cwd=w,stdout=f,stderr=subprocess.STDOUT)
 assert r.returncode!=0
 assert 'FOV read recalculated saved direction' in (out/'read-recalculates-red.log').read_text()
finally:p.write_text(base)
with (out/'read-recalculates-restored.log').open('w') as f:r=subprocess.run(cmd,cwd=w,stdout=f,stderr=subprocess.STDOUT)
assert r.returncode==0
print('read-recalculates red -> green')
