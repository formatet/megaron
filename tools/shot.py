#!/usr/bin/env python3
"""Skärmdump av en showcase-rigg. En etikett per iteration:

    python3 tools/shot.py baseline                    # terrängriggen
    python3 tools/shot.py plains-base fixture=plains  # terräng, annan fixtur
    python3 tools/shot.py units-base units            # enhetsriggen
    python3 tools/shot.py glyph-base glyphs           # glyfriggen
    python3 tools/shot.py flaggor glyphs:set=flaggor  # rigg:query

Kräver att riggen serveras:  python3 -m http.server 8099 --directory web/

Skriver <etikett>.png i SHOT_DIR (default: nuvarande katalog). Sidan ritar en
FRYST frame, så två körningar utan kodändring ska ge pixelidentiska filer —
det är determinismkontrollen och den är ett krav, inte en förhoppning.
"""
import os
import pathlib
import sys

from playwright.sync_api import sync_playwright

HOST = os.environ.get("SHOT_HOST", "http://localhost:8099")
OUT = pathlib.Path(os.environ.get("SHOT_DIR", "."))

# Viewporten måste rymma #map-root HELT. Enhetsriggen är 980 px bred plus
# body-padding; en för smal viewport klipper elementet, och eftersom kameran
# centreras ur canvas.width hamnar hela fixturen utanför bild (den ritade
# allt-dimma-bilden såg ut som ett renderingsfel men var en viewportbugg).
RIGS = {
    "forest": ("showcase-forest.html", 940, 820),
    "units": ("showcase-units.html", 1040, 1080),
    "glyphs": ("showcase-glyphs.html", 1180, 900),
    "cities": ("showcase-cities.html", 1040, 900),
    # Kustsektionen är samma sida med en annan scen — och en annan duk. Den bor
    # som egen riggnyckel så att viewporten följer med scenen automatiskt: en
    # scen som är bredare än sin viewport klipps, och kameran centreras ur
    # canvas.width, så halva fixturen hamnar utanför bild.
    "coast": ("showcase-cities.html?scene=coast", 1220, 940),
    "cityview": ("showcase-cityview.html", 740, 760),
    "world": ("showcase-world.html", 1600, 900),
}


def main(label, query=""):
    # "<rigg>", "<rigg>:<query>", eller en ren query (= terrängriggen).
    rig, _, qs = query.partition(":")
    if rig not in RIGS:
        rig, qs = "forest", query
    page_name, w, h = RIGS[rig]
    sep = "&" if "?" in page_name else "?"
    url = f"{HOST}/static/{page_name}" + (f"{sep}{qs}" if qs else "")

    OUT.mkdir(parents=True, exist_ok=True)
    with sync_playwright() as p:
        b = p.chromium.launch()
        page = b.new_page(viewport={"width": w, "height": h}, device_scale_factor=1)
        errors = []
        page.on("console", lambda m: errors.append(m.text) if m.type == "error" else None)
        page.on("pageerror", lambda e: errors.append(str(e)))
        page.goto(url, wait_until="networkidle")
        page.wait_for_function("window.SHOWCASE && window.SHOWCASE.ready", timeout=5000)
        page.locator("#map-root").screenshot(path=str(OUT / f"{label}.png"))
        b.close()
    for e in errors:
        print("KONSOLFEL:", e, file=sys.stderr)
    print(OUT / f"{label}.png")
    return 1 if errors else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1] if len(sys.argv) > 1 else "shot",
                  sys.argv[2] if len(sys.argv) > 2 else ""))
