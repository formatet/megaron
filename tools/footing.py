#!/usr/bin/env python3
"""Vad står staden på? Räknar blått strax under stadsmassans fotlinje.

    python3 tools/footing.py                 # kustscenen i stadsriggen
    python3 tools/footing.py coast:zoom=0.55 # samma scen, annan zoom
    python3 tools/footing.py cities          # ledrutnätet (ska vara noll)

VARFÖR DET HÄR FINNS. Stadsmassan är 62 px mot hexens 44 och terrängblind, så
en kuststad lade sin gård rakt ut på öppet vatten. Ögat ser det direkt, men
"ser det floating ut?" är inte ett mått, och en visuell slice som inte kan
mätas kan inte heller grindas mot en regression. Måttet här är avsiktligt
smalt och bokstavligt: **ligger det havsblått direkt under en fotpixel?**

Vad det mäter: för varje kolumn i massan, färgen på pixlarna 1–3 px under
kolumnens understa byggda pixel (fotlinjen ur renderarens egen `spriteGround`,
levererad av riggens `SHOWCASE.cities()`). En pixel räknas som hav när den är
blåare än den är röd — havet och dess dyning är kartans enda sådana ytor;
sand, puts, jordtak och gräs har alla r > b.

Vad det INTE mäter: om massans ÖVRE del skymmer hav bakom sig. Det är rätt
djupspråk i ¾-elevation (samma som bergstoppar och lövverk) och ska inte
räknas som fel. Måttet gäller marken staden står på, inte siluetten.

Kräver att riggen serveras:  python3 -m http.server 8099 --directory web/
"""
import os
import pathlib
import sys
import tempfile

from PIL import Image
from playwright.sync_api import sync_playwright

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from shot import HOST, RIGS  # noqa: E402  — en enda sanning om riggarnas viewports

# De två logiska pixlarna DIREKT under fotpixeln. Inte fler: en bank med
# fransad kant ska få sluta efter tre pixlar och lämna vatten därbortom — det
# är en smal strand, inte en stad som står i vattnet. Ett utkast probade 1–3 px
# och räknade den fransen som fel, alltså mätte det bankens BREDD och kallade
# det fötter.
PROBE = (0, 1)


def main(target="coast", out=None):
    rig, _, qs = target.partition(":")
    if rig not in RIGS:
        rig, qs = "coast", target
    page_name, w, h = RIGS[rig]
    sep = "&" if "?" in page_name else "?"
    url = f"{HOST}/static/{page_name}" + (f"{sep}{qs}" if qs else "")

    shot = pathlib.Path(out or (tempfile.gettempdir() + "/footing.png"))
    with sync_playwright() as p:
        b = p.chromium.launch()
        page = b.new_page(viewport={"width": w, "height": h}, device_scale_factor=1)
        page.goto(url, wait_until="networkidle")
        page.wait_for_function("window.SHOWCASE && window.SHOWCASE.ready", timeout=15000)
        if not page.evaluate("!!(window.SHOWCASE && window.SHOWCASE.cities)"):
            print(f"{rig}-riggen exponerar ingen SHOWCASE.cities()", file=sys.stderr)
            b.close()
            return 2
        cities = page.evaluate("window.SHOWCASE.cities()")
        page.locator("#map-root").screenshot(path=str(shot))
        b.close()

    img = Image.open(shot).convert("RGB")
    W, H = img.size
    px = img.load()
    total, rows = 0, []
    for c in cities:
        k = c["k"]
        wet = 0
        for i, fy in enumerate(c["foot"]):
            if fy < 0:
                continue
            # Sprite-koordinater är logiska pixlar; skärmen är k gånger så stor.
            x = int(c["sx"] + (i + 0.5) * k)
            y0 = int(c["sy"] + (fy + 1) * k)
            if not (0 <= x < W):
                continue
            for d in PROBE:
                y = y0 + int(d * k)
                if not (0 <= y < H):
                    continue
                r, g, bl = px[x, y]
                if bl > r:
                    wet += 1
                    # Varje siffra ska ha en bild (arbetsregel: mät inte utan
                    # att kunna peka). Märket sitter PÅ den blöta pixeln.
                    for oy_ in range(-1, 2):
                        for ox_ in range(-1, 2):
                            if 0 <= x + ox_ < W and 0 <= y + oy_ < H:
                                px[x + ox_, y + oy_] = (255, 0, 128)
                    break
        total += wet
        rows.append((c["name"], wet, sum(1 for f in c["foot"] if f >= 0)))
    img.save(shot)

    print(f"{url}")
    print(f"  {'stad':24} {'blöta':>6} {'kolumner':>9}")
    for name, wet, cols in rows:
        flag = "  ⚠" if wet else ""
        print(f"  {name:24} {wet:6d} {cols:9d}{flag}")
    print(f"  {'TOTALT':24} {total:6d}")
    print(f"  bild: {shot}")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1] if len(sys.argv) > 1 else "coast",
                  sys.argv[2] if len(sys.argv) > 2 else None))
