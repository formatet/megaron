#!/usr/bin/env python3
"""Placera speldygnstestets vandrande hostar på SAMMA landmassa.

Varför verktyget finns
----------------------
Spawn balanserar över landmassor (mig 124, join.go "prefer the persisted
landmass with fewer settlements+hosts"). Varje TOM landmassa väger 0, så fyra
spelare hamnar på fyra olika öar — uppmätt 2026-08-23: 17/92/228/262 hexar,
37-61 hexar isär. Upptäckt sker på radie 5 (gossip), 8 (orakel), 10
(isolationsvarningen), så de kan aldrig få veta att varandra finns. Utan
kontakt är spak 5 (handelns funnel) omätbar — se megaron_plan_speldygnstest.md
§3, alternativ (b).

Det här är en KASTBAR RIGGÅTGÄRD mot acceptansvärlden, inte spellogik. Den
rör bara units.q/r (hosten och dess eskort) — founder_phase har ingen
position, provinces är tom före grundningen och FOW räknas ur enhetsläget, så
enhetsraden är hela tillståndet.

    tools/speldygn_placera.py                 # störst landmassa, 15 hexars lucka
    tools/speldygn_placera.py --min-dist 20
    tools/speldygn_placera.py --dry-run       # visa bara vad den skulle göra
"""
import argparse, json, subprocess, sys

PROJECT = "megaron-acc"
DC = ["docker", "compose", "-p", PROJECT, "-f", "docker-compose.yml",
      "-f", "docker-compose.acceptance.yml", "exec", "-T", "postgres",
      "psql", "-U", "poleia", "-d", "poleia", "-tAc"]

# Samma terrängfilter som join.go använder för spawn — ingen host ska landa på
# ett berg eller i en flodfåra bara för att riggen flyttade den.
BAD_TERRAIN = ("coastal_sea", "deep_sea", "river", "river_ford",
               "mountain_limestone", "mountain_red", "semi_desert")


def psql(sql):
    out = subprocess.run(DC + [sql], capture_output=True, text=True, cwd=ROOT)
    if out.returncode != 0:
        sys.exit("psql: " + out.stderr.strip())
    return [l for l in out.stdout.strip().split("\n") if l]


def dist(a, b):
    dq, dr = a[0] - b[0], a[1] - b[1]
    return (abs(dq) + abs(dq + dr) + abs(dr)) // 2


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--landmass", default="auto", help="landmass_id, eller auto = störst")
    ap.add_argument("--min-dist", type=int, default=8,
                    help="minsta godtagbara hexavstånd mellan två hostar. 8 är golvet, inte målet: catchment är radie 2, så under ~6 hexar börjar två städer slåss om samma mark. En 60x60-karta ger i praktiken 9-17 hexar för fyra hostar (uppmätt 2026-08-23)")
    ap.add_argument("--dry-run", action="store_true")
    args = ap.parse_args()

    world = psql("SELECT id FROM worlds ORDER BY created_at DESC LIMIT 1")[0]
    hosts = [r.split("|") for r in psql(
        f"SELECT pl.username, u.owner_id FROM units u JOIN players pl ON pl.id=u.owner_id "
        f"WHERE u.world_id='{world}' AND u.type='nomadic_host' ORDER BY pl.username")]
    if not hosts:
        sys.exit("inga hostar i världen — kör tools/acceptance.sh player NAMN först")

    def sites_on(lm):
        """Girigt urval på EN landmassa: börja i den hex som ligger längst från
        landmassans mitt, ta sedan varje gång den hex som maximerar avståndet
        till redan valda platser. Jämnt spritt slår slumpat — vi vill ha samma
        lucka mellan ALLA par, inte två grannar och två utbölingar."""
        bad = ",".join(f"'{t}'" for t in BAD_TERRAIN)
        tiles = [tuple(int(x) for x in r.split("|")) for r in psql(
            f"SELECT q, r FROM map_tiles WHERE world_id='{world}' AND landmass_id={lm} "
            f"AND terrain NOT IN ({bad}) ORDER BY q, r")]
        if len(tiles) < len(hosts):
            return None, tiles
        cq = sum(t[0] for t in tiles) / len(tiles)
        cr = sum(t[1] for t in tiles) / len(tiles)
        picked = [max(tiles, key=lambda t: abs(t[0] - cq) + abs(t[1] - cr))]
        while len(picked) < len(hosts):
            nxt = max(tiles, key=lambda t: min(dist(t, c) for c in picked))
            if nxt in picked:
                break
            picked.append(nxt)
        return picked, tiles

    # Den STÖRSTA landmassan är inte alltid den rymligaste: en lång smal ö med
    # 200 hexar kan tvinga ihop fyra hostar tätare än en rund med 150. Mät i
    # stället luckan de faktiskt får och ta den ö som ger störst minsta lucka —
    # annars degraderas körningen tyst till grannar som delar catchment.
    if args.landmass == "auto":
        cands = [r.split("|")[0] for r in psql(
            f"SELECT landmass_id, count(*) FROM map_tiles WHERE world_id='{world}' "
            f"AND landmass_id IS NOT NULL GROUP BY 1 ORDER BY count(*) DESC LIMIT 4")]
    else:
        cands = [args.landmass]

    best = (None, None, -1)
    for lm in cands:
        picked, tiles = sites_on(lm)
        if not picked:
            continue
        tightest = min(dist(picked[a], picked[b])
                       for a in range(len(picked)) for b in range(a + 1, len(picked))) \
            if len(picked) > 1 else 0
        print(f"  landmassa {lm}: {len(tiles)} dugliga hexar → tätaste par {tightest}")
        if tightest > best[2]:
            best = (lm, picked, tightest)

    lm, chosen, _ = best
    if not chosen:
        sys.exit(f"ingen landmassa rymmer {len(hosts)} hostar")
    tiles = sites_on(lm)[1]

    pairs = [(a, b, dist(chosen[a], chosen[b]))
             for a in range(len(chosen)) for b in range(a + 1, len(chosen))]
    tight = min(p[2] for p in pairs) if pairs else 0

    print(f"\nvärld {world}  ·  vald landmassa {lm} ({len(tiles)} dugliga hexar)")
    for (name, _), site in zip(hosts, chosen):
        print(f"  {name:<12} → ({site[0]},{site[1]})")
    print("  avstånd:", "  ".join(f"{hosts[a][0][:3]}–{hosts[b][0][:3]} {d}" for a, b, d in pairs))
    if tight < args.min_dist:
        print(f"  ⚠ tätaste paret {tight} hexar < --min-dist {args.min_dist} "
              f"— landmassan rymmer inte fler hostar med den luckan")

    if args.dry_run:
        print("\n(dry-run — inget flyttat)")
        return

    for (name, owner), (q, r) in zip(hosts, chosen):
        psql(f"UPDATE units SET q={q}, r={r}, updated_at=now() "
             f"WHERE world_id='{world}' AND owner_id='{owner}' AND status='positioned'")
    print("\nflyttade — hostar och eskorter står nu på samma landmassa")


if __name__ == "__main__":
    ROOT = subprocess.run(["git", "rev-parse", "--show-toplevel"],
                          capture_output=True, text=True).stdout.strip()
    main()
