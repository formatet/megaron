#!/usr/bin/env python3
"""Minoernas intro — inloggningsskärmens loop (Timothy 2026-09-25).

Beslut (megaron_styrande_beslut): D frygisk, syntes i orderhornets röst, mer bas. Spelas bara på
inloggningssidan och loopar; vid inloggning spelar kartsidan klart cykeln, sedan vardagsrepertoaren.
Därför är filen en SÖMLÖS loop: slutets efterklang viks in över början, havet går runt skarven.

Form (takt à 6/8, punkterad fjärdedel = 76):
  hamnen 4 · motivet 8 · horisonterna (kvintsprång som klättrar) 8 · delfinerna 8 ·
  härligheten (B♭–C–D-dur, galopp, hornackord) 8 · motivet en oktav upp 8 · kadens E♭→D-dur 4 · andning 2

Kör: <venv med numpy scipy mido soundfile>/python tools/music/minoan_intro.py
Ut:  tools/music/prov/minoan_intro.{abc,mid,ogg}
"""
import os, subprocess
import numpy as np, mido, soundfile as sf
from intro_prov import aulos, lyre, horn, drum, lp, abc_note, LET, SR, rng
from scipy.signal import fftconvolve

HERE = os.path.dirname(os.path.abspath(__file__))
OUT = os.path.join(HERE, 'prov')
QPM = 76
EIGHTH = 60 / (QPM * 3)
BAR = 6 * EIGHTH

# (namn, melodi, grundtoner, basmönster, trummönster, hornackord?, dynamik)
# Bokstäver skrivs "vita"; K:Dphr gör B→B♭ och E→E♭. ^F = F♯ (D-dur i härligheten).
S = [
    ("hamnen",  ["z6"] * 4, "DDDD", "pedal", None, False, "mp"),
    ("motivet", ["D3 A3", "B2A G2A", "F2G A2F", "E6", "D3 A3", "B2c d2B", "A2G F2E", "D6"],
                "DBDEDGCD", "calm", "light", False, "mp"),
    ("horisont", ["D3 A3", "F3 c3", "G3 d3", "c6", "B3 f3", "e2d c2B", "c2d e2f", "d6"],
                "DFGCBEFD", "gallop", "gallop", True, "mf"),
    ("delfiner", ["d6", "z2 A/B/c/d/ e2", "d3 c3", "z2 G/A/B/c/ d2", "f6", "z2 c/d/e/f/ g2", "f3 e3", "d6"],
                "BCBGDECD", "long", "sistrum", False, "mf"),
    ("härlighet", ["f3 d3", "e3 g3", "^f6", "a3 d'3", "c'3 b3", "a2g f2e", "f3 e3", "d6"],
                "BCMMEDCM", "gallop", "full", True, "f"),
    ("motivet2", ["d3 a3", "b2a g2a", "f2g a2f", "e6", "d3 a3", "b2c' d'2b", "a2g f2e", "d6"],
                "DBDEDGCD", "gallop", "full", True, "f"),
    ("kadens",  ["e6", "d6", "z6", "z6"], "EMDD", "calm", "end", True, "f"),
    ("andning", ["z6", "z6"], "DD", "pedal", None, False, "p"),
]
# M = D-dur (grundton D, ters F♯). Aldrig A som grundton: A–E♭ är ingen ren kvint.

def root_letter(r):
    return 'D' if r == 'M' else r

def lyre_bar(r):
    root = root_letter(r); i = LET.index(root); f = LET[(i + 4) % 7]
    ro = 3 if i <= LET.index('G') else 2
    fo = ro + (1 if (i + 4) >= 7 else 0)
    a, b, c, d = abc_note(root, ro), abc_note(f, fo), abc_note(root, ro + 1), abc_note(f, fo + 1)
    return f"{a}{b}{c} {d}{c}{b}"

def bass_bar(r, style):
    n = abc_note(root_letter(r), 2); o = abc_note(root_letter(r), 3)
    return {"pedal": f"{n}3 {n}3", "calm": f"{n}3 {o}3", "long": f"{n}6",
            "gallop": f"{n}2{n} {o}2{n}"}[style]

def pad_bar(r):
    root = root_letter(r); i = LET.index(root)
    third = "^F," if r == 'M' else abc_note(LET[(i + 2) % 7], 3 + (1 if i + 2 >= 7 else 0))
    fifth = abc_note(LET[(i + 4) % 7], 3 + (1 if i + 4 >= 7 else 0))
    return f"[{abc_note(root, 3)}{third}{fifth}]6"

DRUMS = {"light": "F,,3 A,,3", "gallop": "F,,2A,, A,,2A,,", "sistrum": "z2^F, z2^F,",
         "full": "[F,,^F,]2A,, [F,,^F,]2A,,", "end": "F,,6"}

def build_abc():
    mel, lyr, bas, pad, drm, hrn, drn = [], [], [], [], [], [], []
    for name, m, roots, bstyle, dstyle, pads, dyn in S:
        for k, (bar, r) in enumerate(zip(m, roots)):
            first = k == 0
            mel.append((f"!{dyn}!" if first else "") + bar)
            lyr.append(("!mp!" if first else "") + lyre_bar(r))
            bas.append((f"!{dyn}!" if first else "") + bass_bar(r, bstyle))
            pad.append(((f"!{'mp' if dyn == 'mf' else dyn}!" if first else "") + pad_bar(r)) if pads else "z6")
            if dstyle == "end":
                drm.append(DRUMS["end"] if k < 2 else "z6")
            else:
                drm.append(DRUMS[dstyle] if dstyle else "z6")
            drn.append("z6")
    n = len(mel)
    # Hornets kvint D–A öppnar (takt 1 och 3) — orderhornets egen signal.
    hrn = ["z6"] * n; hrn[0] = "!mf!D,3 A,3"; hrn[2] = "!mp!D,3 A,3"
    # Aulos andra pipa: en drönare under motivets båda framträdanden.
    starts = {}; i = 0
    for name, m, *_ in S:
        starts[name] = i; i += len(m)
    for nm, dyn in (("motivet", "pp"), ("motivet2", "p")):
        s0 = starts[nm]
        for k in range(8):
            drn[s0 + k] = (f"!{dyn}!" if k == 0 else "") + ("D6-" if k < 7 else "D6")
    def v(bars): return "|".join(bars) + "|]"
    return f"""X:1
T:Minoernas intro (loop)
M:6/8
L:1/8
Q:3/8={QPM}
K:Dphr
V:1 name="aulos"
%%MIDI program 68
{v(mel)}
V:2 name="aulos drönare"
%%MIDI program 68
{v(drn)}
V:3 name="lyra"
%%MIDI program 46
{v(lyr)}
V:4 name="horn"
%%MIDI program 60
{v(hrn)}
V:5 name="hornackord"
%%MIDI program 61
{v(pad)}
V:6 name="bas"
%%MIDI program 33
{v(bas)}
V:7 name="trumma+sistrum" clef=perc
%%MIDI channel 10
[K:C]{v(drm)}
""", n

def bass(f, dur, vel):
    n = int((dur + 0.1) * SR); t = np.arange(n) / SR
    ph = f * t
    x = lp(2 * (ph % 1.0) - 1, 380) + 0.8 * np.sin(2 * np.pi * ph)
    e = np.minimum(1, t / 0.01) * np.exp(-t / max(0.25, dur * 0.8))
    e[-int(0.1 * SR):] *= np.linspace(1, 0, int(0.1 * SR))
    return x * e * vel

VOICES = {68: (aulos, 0.30), 46: (lyre, 0.22), 60: (horn, 0.42), 61: (horn, 0.13), 33: (bass, 0.42)}

def render(mid_path, loop_s):
    t = 0.0; on = {}; notes = []; prog = {}
    for msg in mido.MidiFile(mid_path):
        t += msg.time
        if msg.type == 'program_change':
            prog[msg.channel] = msg.program
        elif msg.type == 'note_on' and msg.velocity > 0:
            on.setdefault((msg.channel, msg.note), []).append((t, msg.velocity))
        elif msg.type in ('note_off', 'note_on'):
            st = on.get((msg.channel, msg.note))
            if st:
                t0, v = st.pop(0); notes.append((msg.channel, msg.note, t0, t - t0, v / 127))
    mix = np.zeros(int((loop_s + 4) * SR))
    for ch, note, t0, dur, v in notes:
        f = 440 * 2 ** ((note - 69) / 12)
        if ch == 9:
            x = drum(note, v)
        else:
            fn, g = VOICES.get(prog.get(ch, 68), (aulos, 0.3))
            x = fn(f, dur, v) * g
        i = int(t0 * SR); mix[i:i + len(x)] += x[:len(mix) - i]
    return mix

def looped(x, L):
    """Vik allt efter loopgränsen in över början — efterklangen ringer över skarven."""
    y = x[:L].copy(); tail = x[L:]
    y[:len(tail)] += tail[:L]
    return y

def sea_loop(L):
    # Brunt brus, loopat genom överblandning; svällningen har ett helt antal perioder per loop.
    n = L + 3 * SR
    x = np.cumsum(rng.normal(0, 1, n)); x -= lp(x, 0.5, 1); x = lp(x, 600)
    x /= np.max(np.abs(x[:L])) + 1e-9
    X = 2 * SR; fade = np.linspace(0, 1, X)
    y = x[:L].copy(); y[:X] = x[:X] * fade + x[L:L + X] * (1 - fade)
    t = np.arange(L) / SR; periods = max(1, round((L / SR) / 7.5))
    return y * (0.5 + 0.5 * np.sin(np.pi * periods * t / (L / SR)) ** 2)

def sea_mask(L):
    t = np.arange(L) / SR; loop_s = L / SR
    head = np.clip(1 - (t - 4 * BAR) / (2 * BAR), 0, 1)             # hamnen, tonar ut in i motivet
    tail = np.clip((t - (loop_s - 4 * BAR)) / (2 * BAR), 0, 1)      # tillbaka i kadensen/andningen
    return np.maximum(head, tail)

def main():
    os.makedirs(OUT, exist_ok=True)
    abc, nbars = build_abc()
    ap = os.path.join(OUT, 'minoan_intro.abc'); mp = ap[:-4] + '.mid'
    open(ap, 'w').write(abc)
    subprocess.run(['abc2midi', ap, '-o', mp], check=True, stdout=subprocess.DEVNULL)
    L = int(round(nbars * BAR * SR))
    x = render(mp, L / SR)
    x /= np.max(np.abs(x)) + 1e-9
    n = int(1.8 * SR); ir = rng.normal(0, 1, n) * np.exp(-np.arange(n) / (0.45 * SR))
    ir = lp(ir, 5000); ir /= np.sqrt(np.sum(ir ** 2))
    x = 0.78 * x + 0.22 * fftconvolve(x, ir)[:len(x)]
    y = looped(x, L) + 0.10 * sea_loop(L) * sea_mask(L)
    y = y / (np.max(np.abs(y)) + 1e-9) * 0.89
    wav = os.path.join(OUT, 'minoan_intro.wav'); sf.write(wav, y.astype(np.float32), SR)
    ogg = wav[:-4] + '.ogg'
    subprocess.run(['ffmpeg', '-y', '-loglevel', 'error', '-i', wav, '-ac', '1', '-c:a', 'libvorbis', '-q:a', '4', ogg], check=True)
    os.remove(wav)
    print(f'{nbars} takter, {L / SR:.1f} s → {ogg}')

if __name__ == '__main__':
    main()
