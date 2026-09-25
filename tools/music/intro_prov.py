#!/usr/bin/env python3
"""PROTOTYP — minoernas intro, A/B-skisser (Timothy 2026-09-25). Ersätter inget i web/static/music/.

Samma melodi i två modus (D dorisk / D frygisk — skiljer på B/B♭ och E/E♭) och två klangdräkter:
  sf    = ABC → abc2midi → fluidsynth med MuseScores "MS Basic" (MIT, se LICENSE-noten i utdatamappen)
  synth = samma MIDI renderat med rå oscillatorer i orderhornets röst (sågtand genom lågpass,
          Karplus-Strong-lyra, sfx.js:s trumdunk) — ingen sampling alls.

Motivet: hornets kvint D–A, sedan halvtonsgrannen B–A. Lyrans ackompanjemang är rena kvintdyader,
ett frö från den hurritiska notationens intervallnamn (strängpar), inte en rekonstruktion.

Kör: <venv med numpy scipy mido soundfile>/python tools/music/intro_prov.py
Ut:  tools/music/prov/*.ogg + prov.html (öppna i webbläsaren)
"""
import os, subprocess, sys
import numpy as np, mido, soundfile as sf
from scipy.signal import butter, lfilter, fftconvolve

HERE = os.path.dirname(os.path.abspath(__file__))
OUT = os.path.join(HERE, 'prov')
SF3 = '/usr/share/mscore-4.7/sound/MS Basic.sf3'
SR = 44100
QPM = 72  # punkterade fjärdedelar per minut → en takt ≈ 1,67 s

# ── Kompositionen (bokstäver i D dorisk; tonartstecknet gör frygiskan) ────────
A  = ["D3 A3", "B2A G2A", "F2G A2F", "E6", "D3 A3", "B2c d2B", "A2G F2E", "D6"]
A1 = ["A3 d3", "e2d c2B", "c2d e2f", "e6", "d2e f2e", "d2c B2A", "G2F E2F", "A6"]
B  = ["F3 c3", "d2c A2c", "B2A G2F", "G6", "G3 d3", "e2d c2B", "A2B c2A", "A6"]
A2 = ["d3 a3", "b2a g2a", "f2g a2f", "e6", "d3 a3", "b2c' d'2b", "a2g f2e", "d6"]
INTRO, CODA = 4, 6
# Grundton per takt. Aldrig A eller B: deras kvint är inte ren i båda modus.
R_A  = list("DGDEDGED")
R_A1 = list("DCCEDGCD")
R_B  = list("FFGCGCFF")
ROOTS = list("DDDD") + R_A + R_A1 + R_B + R_A + list("DDDDDD")

LET = "CDEFGAB"

def abc_note(letter, octave):
    if octave >= 5:
        return letter.lower() + "'" * (octave - 5)
    return letter + "," * (4 - octave)

def lyre_bar(root):
    i = LET.index(root)
    f = LET[(i + 4) % 7]
    ro = 3 if i <= LET.index('G') else 2
    fo = ro + (1 if (i + 4) >= 7 else 0)
    r, fi, o, f2 = abc_note(root, ro), abc_note(f, fo), abc_note(root, ro + 1), abc_note(f, fo + 1)
    return f"{r}{fi}{o} {f2}{o}{fi}"

def build_abc(mode):
    rest = "z6"
    mel = [rest] * INTRO + ["!mp!" + A[0]] + A[1:] + ["!mp!" + A1[0]] + A1[1:] + \
          ["!mf!" + B[0]] + B[1:] + ["!f!" + A2[0]] + A2[1:] + [rest] * CODA
    n = len(mel)
    drone = [rest] * INTRO + ["!pp!D6-"] + ["D6-"] * 14 + ["D6"] + [rest] * 8 + \
            ["!p!D6-"] + ["D6-"] * 6 + ["D6"] + [rest] * CODA
    lyre = ["!mp!" + lyre_bar(ROOTS[0])] + [lyre_bar(r) for r in ROOTS[1:]]
    lyre[-3] = "!p!" + lyre[-3]
    horn = [rest] * n
    horn[2] = "!mf!D,3 A,3"
    horn[n - 6] = "!mf!D,3 A,3"
    horn[n - 4] = "!p!D,3 A,3"
    perc = [rest] * n
    s = INTRO + 8
    for k in range(s, s + 8):
        perc[k] = "F,,3 A,,2A,,"
    for k in range(s + 8, s + 24):
        perc[k] = "F,,2^F, A,,2^F,"
    perc[s] = "!mp!" + perc[s]
    assert all(len(v) == n for v in (drone, lyre, horn, perc)), [len(v) for v in (mel, drone, lyre, horn, perc)]
    def v(bars):
        return "|".join(bars) + "|]"
    key = {'dor': 'Ddor', 'phr': 'Dphr'}[mode]
    return f"""X:1
T:Minoernas intro — prov ({mode})
M:6/8
L:1/8
Q:3/8={QPM}
K:{key}
V:1 name="aulos"
%%MIDI program 68
{v(mel)}
V:2 name="aulos drönare"
%%MIDI program 68
{v(drone)}
V:3 name="lyra"
%%MIDI program 46
{v(lyre)}
V:4 name="horn"
%%MIDI program 60
{v(horn)}
V:5 name="trumma+sistrum" clef=perc
%%MIDI channel 10
[K:C]{v(perc)}
"""

# ── Syntesrösten ────────────────────────────────────────────────────────────
rng = np.random.default_rng(7)

def env(n, a, r):
    e = np.ones(n)
    na, nr = min(n, int(a * SR)), min(n, int(r * SR))
    if na: e[:na] = np.linspace(0, 1, na)
    if nr: e[-nr:] *= np.linspace(1, 0, nr)
    return e

def lp(x, hz, order=2):
    b, a = butter(order, hz / (SR / 2))
    return lfilter(b, a, x)

def saw(ph):
    return 2 * (ph % 1.0) - 1

def aulos(f, dur, vel):
    n = int((dur + 0.08) * SR); t = np.arange(n) / SR
    vib = 1 + 0.004 * np.sin(2 * np.pi * 5.2 * t) * np.clip((t - 0.15) / 0.2, 0, 1)
    ph = np.cumsum(f * vib) / SR
    x = 0.6 * saw(ph) + 0.4 * np.sign(np.sin(2 * np.pi * ph * 1.003))
    return lp(x, 2200) * env(n, 0.03, 0.08) * vel

def lyre(f, dur, vel):
    n = int((dur + 1.4) * SR); p = max(2, int(SR / f))
    buf = rng.uniform(-1, 1, p); chunks = []
    for _ in range(n // p + 1):  # Karplus-Strong, en period i taget
        chunks.append(buf)
        buf = 0.4985 * (buf + np.roll(buf, -1))
    out = np.concatenate(chunks)[:n]
    return lp(out, 3500) * env(n, 0.002, 0.05) * vel

def horn(f, dur, vel):
    n = int((dur + 0.1) * SR); t = np.arange(n) / SR
    bend = np.minimum(1.0, 1 / 1.12 + (1 - 1 / 1.12) * t / (0.25))  # glider upp in i tonen, som ordersignalen
    ph = np.cumsum(f * bend) / SR
    x = lp(saw(ph), 1100) + 0.5 * lp(2 * np.abs(saw(ph / 2)) - 1, 700)
    return x * env(n, 0.05, 0.25) * vel

def drum(note, vel):
    if note in (41, 45):
        d = 0.35; n = int(d * SR); t = np.arange(n) / SR
        f0, f1 = (92, 55) if note == 41 else (140, 90)
        f = f0 * (f1 / f0) ** (t / d)
        return np.sin(2 * np.pi * np.cumsum(f) / SR) * np.exp(-t / 0.09) * vel * 1.3
    # sistrum: tre snabba skrammel
    n = int(0.22 * SR); x = np.zeros(n)
    for k, off in enumerate((0, 0.045, 0.09)):
        m = int(0.05 * SR); i0 = int(off * SR)
        x[i0:i0 + m] += rng.uniform(-1, 1, m) * np.exp(-np.arange(m) / (0.012 * SR)) * (1 - 0.25 * k)
    b, a = butter(2, [5000 / (SR / 2), 11000 / (SR / 2)], 'band')
    return lfilter(b, a, x) * vel * 0.9

def render_synth(mid_path):
    mid = mido.MidiFile(mid_path)
    t = 0.0; on = {}; notes = []; prog = {}
    for msg in mid:
        t += msg.time
        if msg.type == 'program_change':
            prog[msg.channel] = msg.program
        elif msg.type == 'note_on' and msg.velocity > 0:
            on[(msg.channel, msg.note)] = (t, msg.velocity)
        elif msg.type in ('note_off', 'note_on'):
            k = (msg.channel, msg.note)
            if k in on:
                t0, v = on.pop(k); notes.append((msg.channel, msg.note, t0, t - t0, v / 127))
    total = t + 3
    mix = np.zeros(int(total * SR) + SR)
    for ch, note, t0, dur, v in notes:
        f = 440 * 2 ** ((note - 69) / 12)
        if ch == 9: x = drum(note, v)
        else:
            p = prog.get(ch, 0)
            x = {68: aulos, 46: lyre, 60: horn}.get(p, aulos)(f, dur, v)
            x = x * {68: 0.30, 46: 0.22, 60: 0.40}.get(p, 0.3)
        i = int(t0 * SR); mix[i:i + len(x)] += x[:len(mix) - i]
    return mix

# ── Gemensamt: havet, rum, mastring ─────────────────────────────────────────
def sea(n):
    x = np.cumsum(rng.normal(0, 1, n)); x -= lp(x, 0.5, 1)  # brunt brus utan likspänning
    x = lp(x, 600) ; x /= np.max(np.abs(x)) + 1e-9
    t = np.arange(n) / SR
    swell = 0.5 + 0.5 * np.sin(2 * np.pi * t / 7.5) ** 2
    return x * swell

def sea_mask(n):
    bar = 2 * 60 / QPM
    t = np.arange(n) / SR
    m = np.clip(1 - (t - (INTRO + 1) * bar) / (2 * bar), 0, 1)          # tonar ut när melodin kommer
    end = (INTRO + 32) * bar
    m = np.maximum(m, np.clip((t - end) / (2 * bar), 0, 1))                # tillbaka i codan
    return m * np.clip(t / 2.0, 0, 1)

def room(x, wet=0.22):
    n = int(1.8 * SR); ir = rng.normal(0, 1, n) * np.exp(-np.arange(n) / (0.45 * SR))
    ir = lp(ir, 5000); ir /= np.sqrt(np.sum(ir ** 2))
    return (1 - wet) * x + wet * fftconvolve(x, ir)[:len(x)]

def master(x, path):
    x = x / (np.max(np.abs(x)) + 1e-9) * 0.89
    nz = np.nonzero(np.abs(x) > 1e-4)[0]
    x = x[: nz[-1] + SR // 2] if len(nz) else x
    x[-SR // 2:] *= np.linspace(1, 0, min(len(x), SR // 2))
    wav = path[:-4] + '.wav'
    sf.write(wav, x.astype(np.float32), SR)
    subprocess.run(['ffmpeg', '-y', '-loglevel', 'error', '-i', wav, '-ac', '1', '-c:a', 'libvorbis', '-q:a', '4', path], check=True)
    os.remove(wav)

def main():
    os.makedirs(OUT, exist_ok=True)
    for mode in ('dor', 'phr'):
        abc = os.path.join(OUT, f'intro_{mode}.abc'); mid = abc[:-4] + '.mid'
        open(abc, 'w').write(build_abc(mode))
        subprocess.run(['abc2midi', abc, '-o', mid], check=True, stdout=subprocess.DEVNULL)
        # soundfont
        wav = abc[:-4] + '_sf.wav'
        subprocess.run(['fluidsynth', '-ni', '-g', '0.6', '-F', wav, '-r', str(SR), SF3, mid], check=True, stdout=subprocess.DEVNULL)
        x, _ = sf.read(wav); x = x.mean(axis=1) if x.ndim > 1 else x; os.remove(wav)
        x = x / (np.max(np.abs(x)) + 1e-9)
        master(x + 0.10 * sea(len(x)) * sea_mask(len(x)), os.path.join(OUT, f'intro_{mode}_sf.ogg'))
        # syntes
        y = render_synth(mid); y = y / (np.max(np.abs(y)) + 1e-9)
        y = room(y) + 0.10 * sea(len(y)) * sea_mask(len(y))
        master(y, os.path.join(OUT, f'intro_{mode}_synth.ogg'))
        print('klar:', mode)

if __name__ == '__main__':
    main()
