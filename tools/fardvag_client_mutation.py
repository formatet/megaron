#!/usr/bin/env python3
"""Physical forward-route, LIVE gate, privacy and agreed-recipient mutations."""
import pathlib,subprocess
ROOT=pathlib.Path(__file__).resolve().parents[1];OUT=ROOT/'docs/reviews/fardvag'
def mutation(label,file,old,new,expected):
    source=ROOT/file;original=source.read_bytes()
    try:
        assert original.decode().count(old)==1
        source.write_text(original.decode().replace(old,new,1))
        result=subprocess.run(['node','--test','web/static/js/megaron/render/routes.test.mjs','web/static/js/megaron/render/map.test.mjs'],cwd=ROOT,text=True,capture_output=True)
        output=result.stdout+result.stderr;(OUT/('mutation-client-'+label+'.log')).write_text(output)
        assert result.returncode and expected in output,output
        print(label+': named red')
    finally:source.write_bytes(original)
route='web/static/js/megaron/render/routes.js'
mutation('behind-actor',route,'const pos=routePosition(path,progress,hexPx,costs,durationTicks);','const pos=routePosition(path,0,hexPx,costs,durationTicks);','remaining route starts at the actor')
mutation('live-gate',route,'if(live(points[i-1].q,points[i-1].r)&&live(points[i].q,points[i].r))','if(true)','movement marks leaked onto remembered terrain')
mutation('foreign-route-read',route,"export function foreignArrow(actor,hexPx,live,kind='army'){","export function foreignArrow(actor,hexPx,live,kind='army'){\n const privatePath=actor.path;",'foreign route leaked into drawing')
mutation('agreed-recipient','web/static/js/megaron/render/map.js','(t.mine||t.route_known===true)?own(t,','t.mine?own(t,','agreed recipient caravan lost its forward route')
with (OUT/'restored-client.log').open('w') as log:
    subprocess.run(['node','--test','web/static/js/megaron/render/routes.test.mjs','web/static/js/megaron/render/map.test.mjs','web/static/js/megaron/world_time.test.mjs','web/static/js/megaron/ui/hover.test.mjs'],cwd=ROOT,stdout=log,stderr=subprocess.STDOUT,check=True)
print('client: byte restored green')
