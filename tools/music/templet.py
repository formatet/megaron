#!/usr/bin/env python3
"""PROTOTYP — vardagsrepertoarens andra stycke: vördnad/templet (Timothy 2026-09-26). A/B: soundfont kontra syntes.

Rotationsstycke med eget slut, inte en loop (megaron_ljud_minoisk_brief §0). D frygisk rakt av — minoernas
eget modus, utan introts vändning till dur. Hornets kvint D–A blir en liggande drönare; lyran slår glest,
sistrumet (tempelinstrumentet, jfr Skördemannavasen) skakar varannan takt, ramtrumman är ett hjärtslag.
En röst — prästinnan — svarar aulosens bön. Uppenbarelsen är frygiskans egen färg: E♭ som skimrar över D.

Form (takt à 6/8, punkterad fjärdedel = 48):
  åkallan 4 · bönen (motivet utdraget, kadens E♭→D) 8 · svaret (rösten) 8 · uppenbarelsen 4 ·
  återgången 6 · tystnaden 2

Kör: <venv med numpy scipy mido soundfile>/python tools/music/templet.py
Ut:  tools/music/prov/templet.{abc,mid} + templet_sf.ogg + templet_synth.ogg
"""
import os, subprocess
import numpy as np, soundfile as sf
from intro_prov import aulos, lyre, horn, lp, master, room, SF3, SR
from bygget import render_synth

HERE = os.path.dirname(os.path.abspath(__file__))
OUT = os.path.join(HERE, 'prov')
QPM = 48

# (namn, aulos, röst, dynamik). K:Dphr — bokstäverna B och E klingar B♭ och E♭.
S = [
    ("åkallan",     ["z6"] * 4,                                                    ["z6"] * 4, "pp"),
    ("bönen",       ["D6", "A6", "B3 A3", "A6", "G3 F3", "E3 F3", "D6-", "D6"],    ["z6"] * 8, "p"),
    ("svaret",      ["z6"] * 8, ["A3 d3", "e3 d3", "c3 B3", "A6", "B3 c3", "d3 c3", "B3 A3", "A6"], "mp"),
    ("uppenbarelsen", ["z6"] * 4,                                                  ["e6", "d6", "e3 f3", "e6"], "mp"),
    ("återgången",  ["d3 A3", "B3 A3", "G3 F3", "E3 F3", "D6-", "D6"],             ["z6"] * 6, "p"),
    ("tystnaden",   ["z6"] * 2,                                                    ["z6"] * 2, "pp"),
]

def build_abc():
    mel, voc, sec = [], [], []
    for name, m, v, dyn in S:
        for k in range(len(m)):
            mel.append((f"!{dyn}!" if k == 0 and m[k] != "z6" else "") + m[k])
            voc.append((f"!{dyn}!" if k == 0 and v[k] != "z6" else "") + v[k])
            sec.append(name)
    n = len(mel)
    # Drönaren: D–A under allt utom sista takten, ett andetag (ny ansats) var fjärde takt.
    drone = [("!pp!" if i == 0 else "") + ("[D,A,]6-" if (i + 1) % 4 and i < n - 2 else "[D,A,]6") for i in range(n - 1)] + ["z6"]
    # Lyran: en kvintdyad på ettan, bara där något sjungs eller bes.
    lyr = [("[D,A,]6" if s not in ("åkallan", "tystnaden") and i % 2 == 0 else "z6") for i, s in enumerate(sec)]
    lyr[4] = "!p!" + lyr[4]
    # Sistrum varannan takt, ramtrumma som hjärtslag i bönen och återgången.
    drm = []
    for i, s in enumerate(sec):
        sis = "^F," if i % 2 == 1 and s != "tystnaden" else ""
        beat = s in ("bönen", "återgången")
        if beat and sis: drm.append("[F,,^F,]3 F,,3")
        elif beat: drm.append("F,,3 F,,3")
        elif sis: drm.append("^F,6")
        else: drm.append("z6")
    drm[1] = "!pp!" + drm[1]
    hrn = ["z6"] * n
    hrn[0] = "!p!D,3 A,3"
    hrn[n - 2] = "!pp!D,3 A,3"
    def v(bars): return "|".join(bars) + "|]"
    return f"""X:1
T:Vördnad (minoerna)
M:6/8
L:1/8
Q:3/8={QPM}
K:Dphr
V:1 name="aulos"
%%MIDI program 68
{v(mel)}
V:2 name="prästinnan"
%%MIDI program 52
{v(voc)}
V:3 name="drönare"
%%MIDI program 68
{v(drone)}
V:4 name="lyra"
%%MIDI program 46
{v(lyr)}
V:5 name="horn"
%%MIDI program 60
{v(hrn)}
V:6 name="trumma+sistrum" clef=perc
%%MIDI channel 10
[K:C]{v(drm)}
"""

def voice(f, dur, vel):
    """Prästinnan: övertonsrik ton genom två formanter (vokalen 'a'), långsam insats och vibrato."""
    n = int((dur + 0.3) * SR); t = np.arange(n) / SR
    vib = 1 + 0.006 * np.sin(2 * np.pi * 5.0 * t) * np.clip((t - 0.4) / 0.6, 0, 1)
    ph = np.cumsum(f * vib) / SR
    x = sum(np.sin(2 * np.pi * k * ph) / k for k in range(1, 12))
    from scipy.signal import butter, lfilter
    y = 0
    for fc, g in ((800, 1.0), (1150, 0.6), (2900, 0.15)):
        b, a = butter(2, [fc * 0.8 / (SR / 2), fc * 1.2 / (SR / 2)], 'band')
        y = y + g * lfilter(b, a, x)
    e = np.minimum(1, t / 0.35); e[-int(0.3 * SR):] *= np.linspace(1, 0, int(0.3 * SR))
    return y * e * vel

def drone(f, dur, vel):
    """Aulosens andra pipa, mörkare och utan vibrato."""
    n = int((dur + 0.4) * SR); t = np.arange(n) / SR
    x = lp(2 * ((f * t) % 1.0) - 1, 700)
    e = np.minimum(1, t / 0.8); e[-int(0.4 * SR):] *= np.linspace(1, 0, int(0.4 * SR))
    return x * e * vel

# Drönaren och aulosen delar MIDI-program 68 — i syntesen skiljs de åt på kanalordningen nedan.
VOICES = {68: (aulos, 0.30), 52: (voice, 0.55), 46: (lyre, 0.30), 60: (horn, 0.30)}

def main():
    os.makedirs(OUT, exist_ok=True)
    ap = os.path.join(OUT, 'templet.abc'); mp = ap[:-4] + '.mid'
    abc = build_abc()
    open(ap, 'w').write(abc)
    # Syntesen ska ha drönaren i sin egen röst: ge den program 69 bara i syntesens MIDI.
    open(ap[:-4] + '_synth.abc', 'w').write(abc.replace('name="drönare"\n%%MIDI program 68', 'name="drönare"\n%%MIDI program 69'))
    subprocess.run(['abc2midi', ap, '-o', mp], check=True, stdout=subprocess.DEVNULL)
    ms = mp[:-4] + '_synth.mid'
    subprocess.run(['abc2midi', ap[:-4] + '_synth.abc', '-o', ms], check=True, stdout=subprocess.DEVNULL)
    os.remove(ap[:-4] + '_synth.abc')
    wav = ap[:-4] + '_sf.wav'
    subprocess.run(['fluidsynth', '-ni', '-g', '0.6', '-F', wav, '-r', str(SR), SF3, mp], check=True, stdout=subprocess.DEVNULL)
    x, _ = sf.read(wav); x = x.mean(axis=1) if x.ndim > 1 else x; os.remove(wav)
    master(room(x / (np.max(np.abs(x)) + 1e-9), 0.18), os.path.join(OUT, 'templet_sf.ogg'))
    y = render_synth(ms, {**VOICES, 69: (drone, 0.16)}); os.remove(ms)
    master(room(y / (np.max(np.abs(y)) + 1e-9), 0.32), os.path.join(OUT, 'templet_synth.ogg'))
    for k in ('sf', 'synth'):
        p = os.path.join(OUT, f'templet_{k}.ogg')
        print(f'{p}: {sf.info(p).duration:.1f} s')

if __name__ == '__main__':
    main()
