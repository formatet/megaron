#!/usr/bin/env python3
"""PROTOTYP — vardagsrepertoarens första stycke: bygget/glädje (Timothy 2026-09-26). A/B: soundfont kontra syntes.

Repertoarkartan (megaron_ljud_minoisk_brief §0): kartans bädd är en rotation av stycken med EGET slut och
tystnad emellan — detta är inte en loop. Colonizations "glädje" (spår 2): 75 s, klar durtonart, ljus och tät.

Tonart: B♭-dur har exakt samma toner som introts D frygisk — samma tonförråd, annat centrum, ljust.
Motivet D–A–B♭–A ärvs ordagrant; A blir ledton som löser upp i B♭. Hornet öppnar med familjens kvint D–A
och svarar i slutet med B♭–F — arbetet är gjort.

Form (takt à 6/8, punkterad fjärdedel = 66):
  morgonen 4 · motivet i dur 8 · verkstaden (trähammare) 8 · skuggan (g-moll, sistrum) 8 ·
  motivet igen 8 · kadens 4

v2 (Timothy 2026-09-26: "för storslaget, långsammare?"): 84 → 66, inga hornackord, ingen galopp,
högst mf. Det som föll var det moderna/orkestrala — lyra, aulos, ramtrumma och sistrum är kvar.

Kör: <venv med numpy scipy mido soundfile>/python tools/music/bygget.py
Ut:  tools/music/prov/bygget.{abc,mid} + bygget_sf.ogg + bygget_synth.ogg
"""
import os, subprocess
import numpy as np, mido, soundfile as sf
from intro_prov import aulos, lyre, horn, drum, lp, master, room, SF3, SR, rng
from minoan_intro import lyre_bar, bass_bar, pad_bar, bass

HERE = os.path.dirname(os.path.abspath(__file__))
OUT = os.path.join(HERE, 'prov')
QPM = 66

# (namn, melodi, grundtoner, basmönster, trummönster, hornackord?, dynamik)
# K:Bb — bokstäverna B och E klingar B♭ och E♭. Aldrig A som grundton (A–E♭ är ingen ren kvint).
S = [
    ("morgonen",  ["z6"] * 4, "BBBB", "pedal", None, False, "mp"),
    ("motivet",   ["d3 A3", "B2A B2c", "d2e f2d", "c6", "d3 A3", "B2c d2e", "f2e d2c", "B6"],
                  "BBBFBEFB", "calm", "light", False, "mp"),
    ("verkstaden", ["f2f g2f", "e2d c3", "d2d e2d", "c2B A3", "B2c d2e", "f2g a2f", "g2f e2c", "d6"],
                  "BEBFBFCB", "calm", "verk", False, "mf"),
    ("skuggan",   ["G3 B3", "d2c B2A", "G2A B2c", "d6", "e3 d3", "c2B A2G", "A2B c2A", "F6"],
                  "GGCGECFF", "long", "sistrum", False, "mp"),
    ("motivet2",  ["d3 A3", "B2A B2c", "d2e f2d", "c6", "d3 f3", "g2f e2d", "c2B A2c", "B6"],
                  "BBBFBEFB", "calm", "verk", False, "mf"),
    ("kadens",    ["d3 c3", "B6", "z6", "z6"], "FBBB", "long", "end", False, "mp"),
]

# e = hög träkloss (GM 76) — hammaren i verkstaden. ^F, = sistrum (tamburin), F,, / A,, = ramtrumma.
DRUMS = {"light": "F,,3 A,,3", "verk": "F,,2e A,,2e", "sistrum": "z2^F, z2^F,",
         "full": "[F,,^F,]2e [A,,^F,]2e", "end": "F,,6"}

def build_abc():
    mel, lyr, bas, pad, drm = [], [], [], [], []
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
    n = len(mel)
    hrn = ["z6"] * n
    hrn[0] = "!mp!D,3 A,3"            # familjens kvint, som introt och orderhornet
    hrn[2] = "!p!D,3 A,3"
    hrn[n - 3] = "!mp!B,,3 F,3"       # svaret: arbetet är gjort
    def v(bars): return "|".join(bars) + "|]"
    return f"""X:1
T:Bygget (minoerna)
M:6/8
L:1/8
Q:3/8={QPM}
K:Bb
V:1 name="aulos"
%%MIDI program 68
{v(mel)}
V:2 name="lyra"
%%MIDI program 46
{v(lyr)}
V:3 name="horn"
%%MIDI program 60
{v(hrn)}
V:4 name="hornackord"
%%MIDI program 61
{v(pad)}
V:5 name="bas"
%%MIDI program 33
{v(bas)}
V:6 name="trumma+sistrum+träkloss" clef=perc
%%MIDI channel 10
[K:C]{v(drm)}
"""

def woodblock(vel):
    n = int(0.08 * SR); t = np.arange(n) / SR
    x = np.sin(2 * np.pi * 1250 * t) + 0.5 * np.sin(2 * np.pi * 2710 * t)
    x += 0.3 * rng.uniform(-1, 1, n) * np.exp(-t / 0.002)
    return x * np.exp(-t / 0.018) * vel * 0.55

VOICES = {68: (aulos, 0.30), 46: (lyre, 0.24), 60: (horn, 0.42), 61: (horn, 0.12), 33: (bass, 0.40)}

def render_synth(mid_path, voices=VOICES):
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
    mix = np.zeros(int((t + 4) * SR))
    for ch, note, t0, dur, v in notes:
        f = 440 * 2 ** ((note - 69) / 12)
        if ch == 9:
            x = woodblock(v) if note in (76, 77) else drum(note, v)
        else:
            fn, g = voices.get(prog.get(ch, 68), (aulos, 0.3))
            x = fn(f, dur, v) * g
        i = int(t0 * SR); mix[i:i + len(x)] += x[:len(mix) - i]
    return mix

def main():
    os.makedirs(OUT, exist_ok=True)
    ap = os.path.join(OUT, 'bygget.abc'); mp = ap[:-4] + '.mid'
    open(ap, 'w').write(build_abc())
    subprocess.run(['abc2midi', ap, '-o', mp], check=True, stdout=subprocess.DEVNULL)
    wav = ap[:-4] + '_sf.wav'
    subprocess.run(['fluidsynth', '-ni', '-g', '0.6', '-F', wav, '-r', str(SR), SF3, mp], check=True, stdout=subprocess.DEVNULL)
    x, _ = sf.read(wav); x = x.mean(axis=1) if x.ndim > 1 else x; os.remove(wav)
    master(x / (np.max(np.abs(x)) + 1e-9), os.path.join(OUT, 'bygget_sf.ogg'))
    y = render_synth(mp); y = room(y / (np.max(np.abs(y)) + 1e-9))
    master(y, os.path.join(OUT, 'bygget_synth.ogg'))
    for k in ('sf', 'synth'):
        p = os.path.join(OUT, f'bygget_{k}.ogg')
        print(f'{p}: {sf.info(p).duration:.1f} s')

if __name__ == '__main__':
    main()
