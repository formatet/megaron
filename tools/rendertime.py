#!/usr/bin/env python3
"""Rendertid per frame och per pass, på en riggfixtur.

    python3 tools/rendertime.py world                 # helvyn, default zoom
    python3 tools/rendertime.py world:zoom=0.3        # minzoom
    python3 tools/rendertime.py world:fixture=fow     # en spelares vy
    python3 tools/rendertime.py forest:fixture=relief # 9x9-fixturen

Arbetsregel 0 (megaron_terrangrendering) kräver att rendertiden mäts före och
efter varje terräng- eller objektändring **med identisk fixtur**. Hittills har
det gjorts för hand i konsolen, vilket är exakt hur en 24x-regression kunde gå
i drift: två körningar med olika fixturer ger ett tal som ser ut som en mätning
och inte är det. Verktyget tvingar fixturen till ett kommandoradsargument, så
"identisk fixtur" blir kontrollerbart efteråt.

Vad det mäter: väggtid för `render()` från `clearRect` till sista passet, median
över N frames efter uppvärmning, plus varje pass median. Vad det INTE mäter:
komposition till skärm, GPU-tid, eller något som händer utanför render() (fetch,
DOM, WS). Median och inte medel: en enda GC-paus flyttar medelvärdet mer än de
ändringar som brukar mätas.

Kräver att riggen serveras:  python3 -m http.server 8099 --directory web/
"""
import json
import os
import sys

from playwright.sync_api import sync_playwright

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from shot import HOST, RIGS  # noqa: E402  — en enda sanning om riggarnas viewports


def main(target="world", n=60):
    rig, _, qs = target.partition(":")
    if rig not in RIGS:
        rig, qs = "forest", target
    page_name, w, h = RIGS[rig]
    url = f"{HOST}/static/{page_name}" + (f"?{qs}" if qs else "")

    with sync_playwright() as p:
        b = p.chromium.launch()
        page = b.new_page(viewport={"width": w, "height": h}, device_scale_factor=1)
        errors = []
        page.on("console", lambda m: errors.append(m.text) if m.type == "error" else None)
        page.on("pageerror", lambda e: errors.append(str(e)))
        page.goto(url, wait_until="networkidle")
        page.wait_for_function("window.SHOWCASE && window.SHOWCASE.ready", timeout=15000)
        if not page.evaluate("!!(window.SHOWCASE && window.SHOWCASE.time)"):
            print(f"{rig}-riggen exponerar ingen SHOWCASE.time() — lägg till den "
                  f"eller mät en rigg som har den", file=sys.stderr)
            b.close()
            return 2
        res = page.evaluate(f"window.SHOWCASE.time({n})")
        b.close()

    for e in errors:
        print("KONSOLFEL:", e, file=sys.stderr)

    print(f"{url}   n={res['n']}")
    print(f"  TOTAL  {res['total']:7.2f} ms   ({1000 / res['total']:.0f} fps om inget annat kostar)")
    for k, v in sorted(res["passes"].items(), key=lambda kv: -kv[1]):
        print(f"  {k:<12} {v:7.2f} ms   {100 * v / res['total']:4.1f} %")
    if os.environ.get("RENDERTIME_JSON"):
        print(json.dumps(res))
    return 1 if errors else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1] if len(sys.argv) > 1 else "world",
                  int(sys.argv[2]) if len(sys.argv) > 2 else 60))
