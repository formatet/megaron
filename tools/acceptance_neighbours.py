#!/usr/bin/env python3
"""Seeda om acceptansvärlden tills de första spelarna delar LANDMASSA.

Varför verktyget finns
──────────────────────
Spawnregeln (api/handlers/join.go) balanserar på **hemisfär** — q<=halfQ mot
q>halfQ — och håller 4 hexars lucka. Den vet ingenting om landmassor. På en
havstung karta betyder det att grannar hamnar på varsin ö och **inte kan nå
varandra till fots alls**: en nomadisk värd är en landenhet utan skepp.

Mätt 2026-08-05 på en 60x40-seed: 8 landmassor, 600 landhexar, och de tre
första wanaxerna på tre olika öar (61, 46 och <22 hexar). Ingen landväg mellan
någon av dem. En playtestvärld där grannarna inte kan mötas testar ingenting
om möten.

Skriptet löser det på den enda ratt som finns utifrån: seeda om tills utfallet
duger. Det ÄR ett plåster — den riktiga frågan (ska spawn balansera på landmassa
i stället för hemisfär?) är en designfråga och bor i megaron_todo.

Bruk
────
    tools/acceptance_neighbours.py Timothy Patroklos Pyrrhos
    tools/acceptance_neighbours.py --försök 12 Timothy Patroklos Pyrrhos

Ordningen är spawnordningen. Vill du ha utfyllnadsspelare som knuffar
hemisfärbalansen (V-Ö-V-Ö-V) — sätt dem i listan med prefixet `~`; de skapas
men räknas inte in i landmassekravet:

    tools/acceptance_neighbours.py Timothy ~Skugga1 Patroklos ~Skugga2 Pyrrhos

Skriptet river världen varje försök. Kör det ALDRIG när någon spelar.
"""

import argparse
import collections
import csv
import io
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
ACC = ROOT / "tools" / "acceptance.sh"

# Vad en landenhet inte kan gå på. Floden är en vägg för landenheter men
# river_ford är en port i den, så vaden räknas som land (mig 108).
WATER = {"coastal_sea", "deep_sea", "river"}
DIRS = [(1, 0), (-1, 0), (0, 1), (0, -1), (1, -1), (-1, 1)]


def acc(*args: str) -> str:
    return subprocess.run([str(ACC), *args], capture_output=True, text=True,
                          check=True).stdout.strip()


def landmasses(world_id: str) -> dict:
    """Karta hex → landmasse-id, för varje hex en landenhet kan stå på."""
    csv_text = acc("psql", f"COPY (SELECT q,r,terrain FROM map_tiles "
                           f"WHERE world_id='{world_id}') TO STDOUT WITH CSV")
    land = set()
    for q, r, terrain in csv.reader(io.StringIO(csv_text)):
        if terrain not in WATER:
            land.add((int(q), int(r)))

    comp, seen = {}, set()
    for start in land:
        if start in seen:
            continue
        cid = len(set(comp.values()))
        queue = collections.deque([start])
        seen.add(start)
        while queue:
            cur = queue.popleft()
            comp[cur] = cid
            for dq, dr in DIRS:
                nxt = (cur[0] + dq, cur[1] + dr)
                if nxt in land and nxt not in seen:
                    seen.add(nxt)
                    queue.append(nxt)
    return comp


def host_positions() -> dict:
    """username → (q,r) för varje aktiv nomadisk värd."""
    rows = acc("psql",
               "SELECT p.username, u.q, u.r FROM units u "
               "JOIN founder_phase fp ON fp.host_unit_id=u.id AND fp.active "
               "JOIN players p ON p.id=u.owner_id")
    out = {}
    for line in rows.splitlines():
        parts = [c.strip() for c in line.split("|")]
        if len(parts) == 3 and parts[1] and parts[2]:
            out[parts[0]] = (int(parts[1]), int(parts[2]))
    return out


def hexdist(a, b) -> int:
    dq, dr = a[0] - b[0], a[1] - b[1]
    return (abs(dq) + abs(dq + dr) + abs(dr)) // 2


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("players", nargs="+",
                    help="spelarnamn i spawnordning; prefix ~ = utfyllnad")
    ap.add_argument("--försök", "--forsok", dest="tries", type=int, default=10)
    args = ap.parse_args()

    names = [p.lstrip("~") for p in args.players]
    required = [p for p in args.players if not p.startswith("~")]
    required = [p.lstrip("~") for p in required]

    for attempt in range(1, args.tries + 1):
        acc("reset")
        world = acc("world")
        for name in names:
            acc("player", name)

        pos = host_positions()
        missing = [n for n in required if n not in pos]
        if missing:
            print(f"  försök {attempt}: {', '.join(missing)} fick ingen värd — seedar om")
            continue

        comp = landmasses(world)
        ids = {n: comp.get(pos[n]) for n in required}
        anchor = required[0]
        together = [n for n in required if ids[n] == ids[anchor] and ids[n] is not None]

        if len(together) == len(required):
            print(f"\n✓ försök {attempt}: alla {len(required)} på samma landmassa")
            print(f"  värld {world}")
            size = sum(1 for v in comp.values() if v == ids[anchor])
            print(f"  landmassan är {size} hexar\n")
            for n in names:
                if n in pos:
                    d = hexdist(pos[n], pos[anchor])
                    tag = "" if n in required else "  (utfyllnad)"
                    same = "samma ö" if comp.get(pos[n]) == ids[anchor] else "annan ö"
                    print(f"  {n:<12} {str(pos[n]):<10} {d:>3} hexar från {anchor}   {same}{tag}")
            return 0

        apart = [n for n in required if n not in together]
        print(f"  försök {attempt}: {', '.join(apart)} hamnade på annan landmassa — seedar om")

    print(f"\n✗ gav upp efter {args.tries} försök.", file=sys.stderr)
    print("  Havsandelen kan vara för hög för att spawn ska kunna samla "
          "spelarna. Sänk MAP-storleken eller höj den — båda ändrar "
          "landmassefördelningen.", file=sys.stderr)
    return 1


if __name__ == "__main__":
    sys.exit(main())
