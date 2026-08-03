#!/usr/bin/env python3
"""Sammanfattar en katalog med mapgen-debug-JSON till en kalibreringstabell.

    go run ./cmd/mapgen-debug -w 230 -h 230 -seed 1 -n 40 -out /tmp/maps 2>/tmp/rej.txt
    python3 tools/mapgenstats.py /tmp/maps
    grep -c reseeding /tmp/rej.txt          # förkastade seeds — se nedan

VARFÖR DET HÄR FINNS. Reseed-grinden kräver att flera seeds mäts och
redovisas innan parametrarna låses, och en rad per karta ur mapgen-debug
räcker inte: det som avgör är fördelningen över seeds, inte en karta.

TVÅ FÄLLOR VERKTYGET FINNS FÖR ATT STÄNGA.

1. `attempts` ur JSON:en LJUGER om reseedkostnaden när flera seeds körs.
   Filnamnet är nycklat på EFFEKTIV seed, så en begärd seed som förkastades
   och rullade vidare skriver över samma fil som den seed den landade på —
   och den sista skrivaren har alltid attempts=1. Reseedfrekvensen måste
   därför räknas ur mapgen-debugs stderr (`grep -c reseeding`), aldrig ur
   den här tabellen. Därav att kolumnen inte finns med nedan.

2. Före/efter på OLIKA seeds är ingen jämförelse. Kör alltid samma
   `-seed`/`-n` före och efter, och jämför på effektiv seed (raderna nedan
   är dedupade på just den).

Kolumnerna: `ced%` är forest_cedar / land — reseed-grindens egen nämnare.
`>=30` är antalet bestånd som är stora nog att läsa som en region man kan
segla till och hålla; `<=9` är dungarna formmålet varnar för. `flod%` är
hela flodfamiljen (river + river_valley + river_ford) av land.
"""
import glob
import json
import statistics as st
import sys


def main(outdir):
    rows = {}
    for f in glob.glob(outdir + "/*.json"):
        m = json.load(open(f))
        rows[m["effective_seed"]] = m
    if not rows:
        print(f"inga .json i {outdir}", file=sys.stderr)
        return 1

    print(f"{'seed':>5} {'land':>6} {'cedar':>6} {'ced%':>6} {'st':>3} {'>=30':>4} {'<=9':>4} "
          f"{'skog%':>7} {'walk':>5} {'flod%':>6}  bestånd")
    agg = {k: [] for k in ("ced", "st", "big", "small", "for", "walk", "riv")}
    for k in sorted(rows):
        m = rows[k]
        # land_fraction är av HELA kartan, så landytan måste tillbaka via w*h.
        land = round(m["land_fraction"] * m["width"] * m["height"])
        sizes = m.get("cedar_stand_sizes") or []
        big = sum(1 for s in sizes if s >= 30)
        small = sum(1 for s in sizes if s <= 9)
        riv = (m["river_tiles"] + m["river_valley_tiles"] + m["river_ford_tiles"]) / land
        print(f"{k:5d} {land:6d} {m['forest_cedar_tiles']:6d} {100 * m['cedar_fraction']:5.2f}% "
              f"{m['cedar_stands']:3d} {big:4d} {small:4d} {100 * m['forest_fraction']:6.2f}% "
              f"{m['largest_landmass_walkable_fraction']:5.3f} {100 * riv:5.2f}%  {sizes}")
        agg["ced"].append(100 * m["cedar_fraction"])
        agg["st"].append(m["cedar_stands"])
        agg["big"].append(big)
        agg["small"].append(small)
        agg["for"].append(100 * m["forest_fraction"])
        agg["walk"].append(m["largest_landmass_walkable_fraction"])
        agg["riv"].append(100 * riv)

    print(f"\nn={len(rows)} distinkta seeds  (reseedfrekvensen står i stderr, inte här "
          f"— se modulens docstring)")
    for key, label in (("ced", "cederandel %"), ("st", "bestånd"), ("big", "bestånd >=30"),
                       ("small", "dungar <=9"), ("for", "skogsandel %"), ("walk", "walkable"),
                       ("riv", "flodandel %")):
        v = agg[key]
        print(f"  {label:16} medel {st.mean(v):6.2f}   min {min(v):6.2f}   max {max(v):6.2f}")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1] if len(sys.argv) > 1 else "."))
