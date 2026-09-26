#!/usr/bin/env python3
"""PROTOTYP — vardagsrepertoarens tredje stycke: havet/melankoli (Timothy 2026-09-26). A/B: soundfont kontra syntes.

Rotationsstycke med eget slut, inte en loop (megaron_ljud_minoisk_brief §0). Colonizations melankoli (spår 3):
klar molltonart, ljus och tät. D frygisk, rörligt: lyrans vågfigur går i jämna åttondelar hela stycket,
aulosen sjunker från A mot D och landar i den frygiska kadensen E♭→D. Mitt i stycket hoppar delfinerna —
uppåtlöp i B♭-dur med sistrum — innan djupet drar ner det igen. Havsbruset bär början och slut och ligger
svagt under allt.

Form (takt à 6/8, punkterad fjärdedel = 58):
  stranden 4 · vågorna 8 · vågorna' 8 · delfinerna 8 · djupet 8 · vågorna 8 · vågorna' 8 · stranden 4

Kör: <venv med numpy scipy mido soundfile>/python tools/music/havet.py
Ut:  tools/music/prov/havet.{abc,mid} + havet_sf.ogg + havet_synth.ogg
"""
import os, subprocess
import numpy as np, soundfile as sf
from intro_prov import sea, master, room, SF3, SR
from minoan_intro import lyre_bar, bass_bar
from bygget import render_synth

HERE = os.path.dirname(os.path.abspath(__file__))
OUT = os.path.join(HERE, 'prov')
QPM = 58
BAR = 2 * 60 / QPM

WAVE  = ["a6", "g3 f3", "e2d c2d", "e6", "d3 c3", "B2A G2A", "B6", "A6"]
WAVE2 = ["a6", "g3 f3", "e2d c2B", "c6", "d3 c3", "B2A G2F", "E6", "D6"]
# (namn, melodi, grundtoner, basmönster, trummönster, dynamik). K:Dphr — B och E klingar B♭ och E♭.
S = [
    ("stranden",   ["z6"] * 4,  "DDDD",     None,   None,      "pp"),
    ("vågorna",    WAVE,        "DBCCBGGD", "long", None,      "p"),
    ("vågorna2",   WAVE2,       "DBCCBGED", "long", "puls",    "mp"),
    ("delfinerna", ["z2 d/e/f/g/ a2", "f3 d3", "z2 c/d/e/f/ g2", "e3 c3", "z2 B/c/d/e/ f2", "d3 B3", "c2d e2c", "d6"],
                   "BBCCBBFB", "calm", "sistrum", "mf"),
    ("djupet",     ["D3 F3", "G3 A3", "B6", "A6", "G3 F3", "E3 F3", "G6", "F6"],
                   "DGGDEEBF", "long", "puls", "mp"),
    ("vågorna3",   WAVE,        "DBCCBGGD", "long", "puls",    "mp"),
    ("vågorna4",   WAVE2,       "DBCCBGED", "long", None,      "p"),
    ("stranden2",  ["z6"] * 4,  "DDDD",     None,   None,      "pp"),
]
DRUMS = {"puls": "F,,3 z3", "sistrum": "z2^F, z2^F,"}

def build_abc():
    mel, lyr, bas, drm = [], [], [], []
    for name, m, roots, bstyle, dstyle, dyn in S:
        for k, (bar, r) in enumerate(zip(m, roots)):
            first = k == 0
            mel.append((f"!{dyn}!" if first and bar != "z6" else "") + bar)
            # Vågfiguren börjar i strandens tredje takt och tystnar i slutets tredje.
            silent = (name == "stranden" and k < 2) or (name == "stranden2" and k >= 2)
            lyr.append("z6" if silent else ((f"!{'p' if dyn in ('pp', 'p') else 'mp'}!" if first else "") + lyre_bar(r)))
            bas.append(((f"!{dyn}!" if first else "") + bass_bar(r, bstyle)) if bstyle else "z6")
            drm.append(DRUMS[dstyle] if dstyle else "z6")
    n = len(mel)
    hrn = ["z6"] * n
    hrn[1] = "!pp!D,3 A,3"        # hornet långt borta över vattnet
    hrn[n - 3] = "!pp!D,3 A,3"
    drm[next(i for i, d in enumerate(drm) if d != "z6")] = "!p!" + drm[next(i for i, d in enumerate(drm) if d != "z6")]
    def v(bars): return "|".join(bars) + "|]"
    return f"""X:1
T:Havet (minoerna)
M:6/8
L:1/8
Q:3/8={QPM}
K:Dphr
V:1 name="aulos"
%%MIDI program 68
{v(mel)}
V:2 name="lyra"
%%MIDI program 46
{v(lyr)}
V:3 name="horn"
%%MIDI program 60
{v(hrn)}
V:4 name="bas"
%%MIDI program 33
{v(bas)}
V:5 name="trumma+sistrum" clef=perc
%%MIDI channel 10
[K:C]{v(drm)}
""", n

def sea_mask(L, nbars):
    """Svagt under allt, fullt i strandens takter i början och slutet."""
    t = np.arange(L) / SR; end = nbars * BAR
    head = np.clip(1 - (t - 3 * BAR) / (2 * BAR), 0, 1)
    tail = np.clip((t - (end - 5 * BAR)) / (2 * BAR), 0, 1)
    return np.maximum(0.3, np.maximum(head, tail)) * np.clip(t / 3.0, 0, 1)

def with_sea(x, nbars):
    x = x / (np.max(np.abs(x)) + 1e-9)
    return x + 0.12 * sea(len(x)) * sea_mask(len(x), nbars)

def main():
    os.makedirs(OUT, exist_ok=True)
    abc, nbars = build_abc()
    ap = os.path.join(OUT, 'havet.abc'); mp = ap[:-4] + '.mid'
    open(ap, 'w').write(abc)
    subprocess.run(['abc2midi', ap, '-o', mp], check=True, stdout=subprocess.DEVNULL)
    wav = ap[:-4] + '_sf.wav'
    subprocess.run(['fluidsynth', '-ni', '-g', '0.6', '-F', wav, '-r', str(SR), SF3, mp], check=True, stdout=subprocess.DEVNULL)
    x, _ = sf.read(wav); x = x.mean(axis=1) if x.ndim > 1 else x; os.remove(wav)
    master(with_sea(room(x / (np.max(np.abs(x)) + 1e-9), 0.2), nbars), os.path.join(OUT, 'havet_sf.ogg'))
    y = render_synth(mp)
    master(with_sea(room(y / (np.max(np.abs(y)) + 1e-9), 0.28), nbars), os.path.join(OUT, 'havet_synth.ogg'))
    for k in ('sf', 'synth'):
        p = os.path.join(OUT, f'havet_{k}.ogg')
        print(f'{p}: {sf.info(p).duration:.1f} s')

if __name__ == '__main__':
    main()
