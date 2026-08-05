#!/usr/bin/env python3
"""deposit_contrast.py — ΔE76-matris för fyndighetsmarkörerna i map.js.

Ren funktion, inga bilder, inga beroenden utöver stdlib. Läser sRGB-hex,
konverterar sRGB → linjär → XYZ (D65) → CIE Lab, och skriver ut ΔE76
(euklidiskt avstånd i Lab) mellan varje par färger.

Bakgrund: `drawDepositIcons` i web/static/js/megaron/render/map.js ritar fyra
fyndighetsmarkörer. Tenn (`sn`, #909090) och silver (`ag`, #C0C8D8) mättes
2026-08-04 till ΔE76 = 22,5 — tre gånger närmare varandra än något annat par
(tenn↔koppar 61,0, silver↔koppar 72,7) och råkar dessutom vara de två
formmässigt mest lika markörerna (rektangel 4×3 vs romb 4×5). Den här riggen
existerar för att döma en ersättningsfärg för tenn (kassiterit, mörkbrun) mot
ALLA grannar den kan möta — inte bara mot silver — innan koden ändras.

Körning:
    python3 tools/deposit_contrast.py                  # baslinje (#909090)
    python3 tools/deposit_contrast.py --tin '#4A3220'   # pröva en kandidat
    python3 tools/deposit_contrast.py --tin '#4A3220' --tin '#5A3A22' ...

Skriver hela matrisen (alla par) plus en separat ΔL*-rad för varje
tenn-kandidat mot silver, eftersom separationen ska ligga i ljushet.
"""
import argparse
import itertools
import sys

# ── sRGB → CIE Lab (D65) ──────────────────────────────────────────────────

def hex_to_rgb(h):
    h = h.lstrip('#')
    return tuple(int(h[i:i + 2], 16) for i in (0, 2, 4))


def srgb_to_linear(c):
    c = c / 255.0
    if c <= 0.04045:
        return c / 12.92
    return ((c + 0.055) / 1.055) ** 2.4


# sRGB → XYZ (D65), IEC 61966-2-1
_M = (
    (0.4124564, 0.3575761, 0.1804375),
    (0.2126729, 0.7151522, 0.0721750),
    (0.0193339, 0.1191920, 0.9503041),
)

# D65 reference white, 2° observer (percent scale, Y=100)
_XN, _YN, _ZN = 95.0489, 100.0, 108.8840


def rgb_to_xyz(rgb):
    r, g, b = (srgb_to_linear(c) for c in rgb)
    x = (_M[0][0] * r + _M[0][1] * g + _M[0][2] * b) * 100.0
    y = (_M[1][0] * r + _M[1][1] * g + _M[1][2] * b) * 100.0
    z = (_M[2][0] * r + _M[2][1] * g + _M[2][2] * b) * 100.0
    return x, y, z


def _f(t):
    delta = 6.0 / 29.0
    if t > delta ** 3:
        return t ** (1.0 / 3.0)
    return t / (3 * delta ** 2) + 4.0 / 29.0


def xyz_to_lab(xyz):
    x, y, z = xyz
    fx, fy, fz = _f(x / _XN), _f(y / _YN), _f(z / _ZN)
    L = 116.0 * fy - 16.0
    a = 500.0 * (fx - fy)
    b = 200.0 * (fy - fz)
    return L, a, b


def hex_to_lab(h):
    return xyz_to_lab(rgb_to_xyz(hex_to_rgb(h)))


def delta_e76(lab1, lab2):
    return sum((c1 - c2) ** 2 for c1, c2 in zip(lab1, lab2)) ** 0.5


# ── Baslinjepaletten, hämtad ur map.js — gissa aldrig ─────────────────────
# Markörfärger: drawDepositIcons, map.js:2500-2540.
# Kontur: samma funktion, ctx.strokeStyle, map.js:2514 (#221E18, CHARCOAL).
# Höjdtoner: PALETTE.mountain_red / PALETTE.hills, map.js:76 och :86 —
# tenn ligger per mapgens sanning på höjd (showcase-forest.html:216-228).
BASE_COLORS = {
    'koppar (cu)':          '#C47C20',
    'ceder (cd)':           '#2A7010',
    'silver (ag)':          '#C0C8D8',
    'kontur (charcoal)':    '#221E18',
    'mountain_red c0':      '#A07A5E',
    'mountain_red c1':      '#82604A',
    'hills c0':             '#C8A464',
    'hills c1':             '#B08C50',
}


def print_matrix(colors):
    names = list(colors.keys())
    labs = {n: hex_to_lab(colors[n]) for n in names}
    width = max(len(n) for n in names) + 2
    header = ' ' * width + ''.join(f'{n[:10]:>12}' for n in names)
    print(header)
    for n1 in names:
        row = f'{n1:<{width}}'
        for n2 in names:
            de = delta_e76(labs[n1], labs[n2])
            row += f'{de:12.1f}'
        print(row)
    print()
    return labs


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument('--tin', action='append', default=None,
                     help="tennfärg att pröva (hex). Kan upprepas. Default: nuläget #909090.")
    args = ap.parse_args()

    tin_candidates = args.tin if args.tin else ['#909090']

    for tin_hex in tin_candidates:
        colors = {'tenn (sn)': tin_hex}
        colors.update(BASE_COLORS)
        print(f'=== tenn = {tin_hex} ===')
        labs = print_matrix(colors)

        tin_lab = labs['tenn (sn)']
        print('Nyckelpar (tenn mot varje granne):')
        for name in BASE_COLORS:
            de = delta_e76(tin_lab, labs[name])
            dl = tin_lab[0] - labs[name][0]
            print(f'  tenn ↔ {name:<22} ΔE76 = {de:6.1f}   ΔL* = {dl:7.1f}')
        print()


if __name__ == '__main__':
    sys.exit(main())
