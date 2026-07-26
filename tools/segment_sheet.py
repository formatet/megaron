#!/usr/bin/env python3
"""Segmentera ett formark (Wanax-sigill, oikosflaggor) till enskilda PNG:er
plus ett INDEXERAT kontaktark, så varje form får ett stabilt id.

    python3 tools/segment_sheet.py <ark.png> <utkatalog> [--threshold N] [--label NAMN]

Räkna aldrig former för hand. Arken ser regelbundna ut och är det inte:
oikosflaggorna gav 10+10+12 = 32 former, inte de ~33 ögat gissade.

id = <label>-r<rad>c<kolumn>, i läsordning. Ordningen är arkets, inte en
sorteringsordning — den ändras bara om arket självt ändras.
"""
import argparse
import pathlib
import subprocess
import sys

sys.path.insert(0, str(pathlib.Path(__file__).parent))
from sheetlib import grid_cells, load_gray  # noqa: E402

# Tröskeln beror på arket: sigillen är svart tusch på pergament (140 räcker),
# flaggorna är bleka konturer och behöver högre tröskel för att synas alls.
DEFAULT_THRESHOLD = {"Wanaxsigill": 140, "oikosflaggor": 200}


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("sheet")
    ap.add_argument("outdir")
    ap.add_argument("--threshold", type=int, default=None)
    ap.add_argument("--label", default=None)
    args = ap.parse_args()

    sheet = pathlib.Path(args.sheet)
    label = args.label or sheet.stem
    thr = args.threshold or DEFAULT_THRESHOLD.get(sheet.stem, 160)
    out = pathlib.Path(args.outdir)
    out.mkdir(parents=True, exist_ok=True)

    w, h, px = load_gray(sheet)
    cells = grid_cells(w, h, px, thr)

    per_row = {}
    for row, _col, *_ in cells:
        per_row[row] = per_row.get(row, 0) + 1

    for row, col, x0, y0, x1, y1 in cells:
        name = f"{label}-r{row}c{col}.png"
        subprocess.run(["magick", str(sheet), "-crop",
                        f"{x1 - x0}x{y1 - y0}+{x0}+{y0}", "+repage",
                        str(out / name)], check=True)

    # Kontaktarket: samma former, numrerade. Det är beviset för att
    # segmenteringen inte slog ihop två grannar eller delade en form i två.
    # `-font` är inte valfritt: ImageMagick hittar ingen defaultfont här och
    # dör med "unable to read font (null)". fc-match ger en som finns.
    font = subprocess.run(["fc-match", "-f", "%{file}", "monospace"],
                          capture_output=True, text=True, check=True).stdout.strip()
    tiles = []
    for row, col, *_ in cells:
        tiles += ["-label", f"r{row}c{col}", str(out / f"{label}-r{row}c{col}.png")]
    contact = out / f"{label}-kontaktark.png"
    subprocess.run(["montage", "-font", font, "-pointsize", "13"] + tiles
                   + ["-background", "white", "-fill", "black",
                      "-tile", f"{max(per_row.values())}x",
                      "-geometry", "80x80+3+3", str(contact)], check=True)

    print(f"{label}: {len(cells)} former i {len(per_row)} rader "
          f"(per rad: {[per_row[r] for r in sorted(per_row)]})")
    print(f"  → {out}/  ·  kontaktark: {contact}")


if __name__ == "__main__":
    main()
