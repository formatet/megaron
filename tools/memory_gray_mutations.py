#!/usr/bin/env python3
"""Physical named assertion red -> restored green for remembered grayscale."""
from pathlib import Path
import subprocess, sys, tempfile
root=Path(__file__).resolve().parents[1]
out=Path(sys.argv[1]);out.mkdir(parents=True,exist_ok=True)
source=root/'web/static/js/megaron/render/memory.js'
original=source.read_text()
mutations=[('tier', "t.tier === 'remembered'", "t.tier !== 'fog'", 'remembered only'),
           ('saturation', "ctx.globalCompositeOperation = 'saturation'", "ctx.globalCompositeOperation = 'source-over'", 'full desaturation preserves destination lightness')]
try:
    for name,before,after,assertion in mutations:
        assert original.count(before)==1
        source.write_text(original.replace(before,after))
        run=subprocess.run(['node','--test','web/static/js/megaron/render/memory.test.mjs'],cwd=root,capture_output=True,text=True)
        (out/(name+'-red.log')).write_text(run.stdout+run.stderr)
        assert run.returncode and assertion in run.stdout, name+' did not fail its named assertion'
        source.write_text(original)
        restored=subprocess.run(['node','--test','web/static/js/megaron/render/memory.test.mjs'],cwd=root,capture_output=True,text=True)
        (out/(name+'-restored.log')).write_text(restored.stdout+restored.stderr)
        assert restored.returncode==0
        print(name+': named assertion red -> restored green')
finally:source.write_text(original)

# End-to-end consumer mutation: a tested helper must actually be called by map.js.
if len(sys.argv)>3:
    renderer=root/'web/static/js/megaron/render/map.js'
    saved=renderer.read_text()
    call="  drawRemembered(ctx, vis, t => { const p = hexPx(t.q, t.r); return hexPts(p.x, p.y); });"
    assert saved.count(call)==1
    try:
        renderer.write_text(saved.replace(call,'  // mutation: grayscale pass removed'))
        with tempfile.TemporaryDirectory(prefix='megaron-gray-mutation-') as temp:
            run=subprocess.run(['python3','tools/memory_gray_replay.py',sys.argv[2],sys.argv[3],temp],cwd=root,capture_output=True,text=True)
        (out/'consumer-red.log').write_text(run.stdout+run.stderr)
        assert run.returncode and "'changedPixels': 0" in run.stderr,'missing grayscale pass did not fail pixel assertion'
        renderer.write_text(saved)
        with tempfile.TemporaryDirectory(prefix='megaron-gray-restored-') as temp:
            restored=subprocess.run(['python3','tools/memory_gray_replay.py',sys.argv[2],sys.argv[3],temp],cwd=root,capture_output=True,text=True)
        (out/'consumer-restored.log').write_text(restored.stdout+restored.stderr)
        assert restored.returncode==0,restored.stderr
        print('consumer: actual pixel assertion red -> restored green')
    finally:renderer.write_text(saved)
