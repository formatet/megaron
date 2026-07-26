#!/usr/bin/env python3
"""Hur många former överlever kartskalan? — mät innan du pixlar.

    python3 tools/discriminability.py <formkatalog> [--sizes 6,8,12,24]

Bakgrunden till frågan: en enhet på målstorlek 14–16 px bär ca 6×6 px
identitetsmärke ≈ 36 pixlar. Om 100 sigill inte går att skilja åt i 36 pixlar
är det en informationsteoretisk gräns, inte ett hantverksproblem — och då ska
ingen rita 100 sprites för den skalan.

Varje form skalas ned till S×S (proportionerna behålls, resten fylls ut) och
**tröskas till 1 bit**. Det är vad renderaren faktiskt gör: hårda pixlar, ingen
kantutjämning (princip 10). Sedan mäts normaliserat hamming-avstånd mellan alla
formpar — andelen pixlar som skiljer.

⚠ Detta mäter form mot form, inte form mot bakgrund. **Diskriminerbarhet**
(kan jag skilja sigill A från sigill B?) är bakgrundsoberoende; **läsbarhet**
(syns märket alls mot kalksten?) är det inte. Den andra frågan ställs i
`web/static/showcase-glyphs.html`, inte här.
"""
import argparse
import json
import pathlib
import subprocess
import sys

sys.path.insert(0, str(pathlib.Path(__file__).parent))
from sheetlib import downsample, fill_silhouette, ink_mask, load_gray  # noqa: E402

# Andel skiljande pixlar. Ingen av dem är kanonisk — hela poängen är att visa
# kurvan så beslutet om tiering fattas på data i stället för på en vald siffra.
THRESHOLDS = (0.0, 0.05, 0.10, 0.20)


def bitmap(path, size, *, fill=False):
    """1-bitars size×size-raster av formen."""
    w, h, px = load_gray(path)
    mask = ink_mask(w, h, px)
    if fill:
        mask = fill_silhouette(w, h, mask)
    return downsample(w, h, mask, size)


def greedy_distinct(names, dist, thr):
    """Största delmängd där VARJE par skiljer sig ≥ thr, girigt i arkordning.
    Girig = undre gräns (exakt maxklick är NP-svårt); rapporteras som sådan."""
    kept = []
    for n in names:
        if all(dist[(min(n, k), max(n, k))] >= thr for k in kept):
            kept.append(n)
    return kept


def dump_rasters(outdir, label, size, names, bits):
    """Skriv rastren som ett uppskalat kontaktark. Siffrorna ensamma ljuger
    lätt: samma tröskling som gav "496 identiska par" var i själva verket
    tomma raster. Ett ark man kan titta på fångar det på en sekund."""
    out = pathlib.Path(outdir)
    out.mkdir(parents=True, exist_ok=True)
    font = subprocess.run(["fc-match", "-f", "%{file}", "monospace"],
                          capture_output=True, text=True, check=True).stdout.strip()
    tiles = []
    for n in names:
        pgm = f"P5\n{size} {size}\n255\n".encode() + \
            bytes(0 if v else 255 for v in bits[n])
        p = out / f"{label}-{size}px-{n}.png"
        subprocess.run(["magick", "pgm:-", "-scale", f"{max(1, 96 // size) * 100}%",
                        str(p)], input=pgm, check=True)
        tiles += ["-label", n.split("-")[-1], str(p)]
    subprocess.run(["montage", "-font", font, "-pointsize", "11"] + tiles
                   + ["-tile", "10x", "-geometry", "+2+2", "-background", "#bbb",
                      str(out / f"{label}-{size}px-ark.png")], check=True)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("glyphdir")
    ap.add_argument("--sizes", default="6,8,12,24")
    ap.add_argument("--fill", action="store_true",
                    help="fyll konturformer till massiv siluett (oikosflaggorna)")
    ap.add_argument("--dump", metavar="KATALOG",
                    help="skriv rastren som PNG — mätningen ska gå att titta på")
    ap.add_argument("--emit-json", metavar="FIL",
                    help="skriv rastren som JSON för showcase-glyphs.html")
    args = ap.parse_args()

    d = pathlib.Path(args.glyphdir)
    files = sorted(p for p in d.glob("*.png") if "kontaktark" not in p.name)
    if not files:
        sys.exit(f"inga former i {d}")
    names = [p.stem for p in files]
    print(f"{d.name}: {len(files)} former\n")
    emitted = {}

    for size in (int(s) for s in args.sizes.split(",")):
        bits = {p.stem: bitmap(p, size, fill=args.fill) for p in files}
        npix = size * size
        if args.dump:
            dump_rasters(args.dump, d.name, size, names, bits)
        if args.emit_json:
            # En rad = ett hex-tal, bit 0 längst till vänster. Kompakt nog att
            # checka in (27 kB för alla 132 former × fyra storlekar) och läsbart
            # nog att felsöka utan verktyg.
            emitted[str(size)] = {
                n: [f"{int(''.join(str(b) for b in bits[n][y * size:(y + 1) * size]) or '0', 2):0{(size + 3) // 4}x}"
                    for y in range(size)]
                for n in names}
        dist = {}
        for i, a in enumerate(names):
            for b in names[i + 1:]:
                ba, bb = bits[a], bits[b]
                dist[(a, b)] = sum(x != y for x, y in zip(ba, bb)) / npix

        pairs = sorted(dist.items(), key=lambda kv: kv[1])
        print(f"── {size}×{size} px ({npix} pixlar) " + "─" * 34)
        for thr in THRESHOLDS:
            below = sum(1 for v in dist.values() if v < thr) if thr else \
                sum(1 for v in dist.values() if v == 0)
            keep = greedy_distinct(names, dist, thr if thr else 1 / npix)
            label = "identiska" if thr == 0 else f"< {thr:.0%} skillnad"
            print(f"   {label:>16}: {below:5d} par  ·  "
                  f"{len(keep):3d}/{len(names)} former parvis distinkta (girig undre gräns)")
        worst = ", ".join(f"{a}~{b} {v:.1%}" for (a, b), v in pairs[:10])
        print(f"   tio värsta paren: {worst}\n")

    if args.emit_json:
        p = pathlib.Path(args.emit_json)
        p.write_text(json.dumps({"set": d.name, "names": names, "sizes": emitted},
                                separators=(",", ":")))
        print(f"→ {p} ({p.stat().st_size // 1024} kB)")


if __name__ == "__main__":
    main()
