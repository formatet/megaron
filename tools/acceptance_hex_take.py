#!/usr/bin/env python3
"""Acceptansfixtur för delad catchment: en hex som grannen håller och som DU kan ta.

Varför verktyget finns
──────────────────────
BILD-grinden för megaron_plan_delad_catchment.md är citygridens *kan tas*-läge.
Det kräver ett läge som en ny värld aldrig ger av sig själv: två städer på
avstånd 3, där stad A har gubbar och sin farm på en hex i det överlappande
bältet, och där B:s Wanax har en enhet i sentry på samma hex.

Vad som går genom riktiga verb: registrering, join, grundning av båda städerna
(B:s grundning på avstånd 3 är i sig ett prov på det nya avståndet), A:s
startfarm och startgubbar (grundningens egen auto-placering), och att B frigör
en gubbe (DELETE …/placements). Två saker skrivs direkt i DB:n, eftersom
de inte är det som prövas: B:s nomadvärd flyttas till en plats på avstånd 3
innan den grundar, och en av B:s enheter ställs `positioned`/`sentry` på hexen.

Bruk (ur worktreen vars kod riggen ska köra):
    tools/acceptance.sh up        # eller reset
    tools/acceptance_hex_take.py

Skriver ut inloggningar, hexen och placement-options för B — allt som behövs
för att öppna B:s stad i webben och se hexen.
"""
import sys
import time

from hexbuildings_fixture import (API, PASSWORD, http, psql, register, world_id,
                                  ensure_world_started, founding_status, settle,
                                  placement_options)

LAND_EXCLUDED = ("coastal_sea", "deep_sea", "river", "river_ford",
                 "mountain_limestone", "mountain_red", "semi_desert")


def dist(a, b):
    dq, dr = a[0] - b[0], a[1] - b[1]
    return (abs(dq) + abs(dr) + abs(dq + dr)) // 2


def join(w, name):
    tok = register(name)
    st, d = http("POST", f"{API}/worlds/{w}/join", bearer=tok)
    if st not in (200, 201):
        sys.exit(f"join {name} failed: {st} {d}")
    return tok


def settlement_of(w, tok):
    st, d = http("GET", f"{API}/worlds/{w}/settlements", bearer=tok)
    if st != 200:
        sys.exit(f"GET settlements failed: {st} {d}")
    ss = d if isinstance(d, list) else d.get("settlements", [])
    if not ss:
        sys.exit("player has no settlement after settle")
    return ss[0]


def main():
    w = world_id()
    stamp = int(time.time())
    name_a, name_b = f"hexA{stamp}", f"hexB{stamp}"
    tok_a, tok_b = join(w, name_a), join(w, name_b)
    ensure_world_started(w)

    settle(w, tok_a, f"Petras{stamp % 1000}")
    sa = settlement_of(w, tok_a)
    sa_id = sa.get("id") or sa["settlement_id"]
    ca = tuple(map(int, psql(
        f"SELECT p.map_q, p.map_r FROM settlements s JOIN provinces p ON p.id = s.province_id "
        f"WHERE s.id = '{sa_id}'").split("|")))
    farm = psql(f"SELECT hex_q, hex_r FROM buildings WHERE settlement_id = '{sa_id}' "
                f"AND building_type = 'farm' LIMIT 1")
    if not farm:
        sys.exit("A got no starter farm — this spawn has no grain hex; re-run after reset")
    x = tuple(map(int, farm.split("|")))
    held = int(psql(f"SELECT count(*) FROM settlement_placement WHERE settlement_id = '{sa_id}' "
                    f"AND target_kind = 'hex' AND hex_q = {x[0]} AND hex_r = {x[1]}") or "0")
    if held == 0:
        sys.exit(f"A has no gubbar on its farm hex {x} — fixture premise broken")

    # B's centre: distance exactly 3 from A, with A's farm hex inside B's catchment.
    rows = psql(f"SELECT q, r FROM map_tiles WHERE world_id = '{w}' "
                f"AND terrain NOT IN ({','.join(repr(t) for t in LAND_EXCLUDED)}) "
                f"AND q BETWEEN {ca[0] - 3} AND {ca[0] + 3} AND r BETWEEN {ca[1] - 3} AND {ca[1] + 3}")
    cands = [tuple(map(int, l.split("|"))) for l in rows.splitlines() if l.strip()]
    cands = [c for c in cands if dist(c, ca) == 3 and dist(c, x) <= 2]
    if not cands:
        sys.exit(f"no land hex at distance 3 from A {ca} reaches A's farm hex {x}; re-run after reset")
    host = psql(f"SELECT fp.host_unit_id FROM founder_phase fp JOIN players pl ON pl.id = fp.owner_id "
                f"WHERE fp.world_id = '{w}' AND pl.username = '{name_b}' AND fp.active")
    for cb in cands:
        psql(f"UPDATE units SET q = {cb[0]}, r = {cb[1]} WHERE id = '{host}'")
        st, d = http("POST", f"{API}/worlds/{w}/founding/settle", {"name": f"Lykos{stamp % 1000}"}, bearer=tok_b)
        if st in (200, 201):
            break
        print(f"  B could not settle at {cb}: {st} {d.get('error', d) if isinstance(d, dict) else d}")
    else:
        sys.exit("B could not settle at any distance-3 site")
    sb = settlement_of(w, tok_b)
    sb_id = sb.get("id") or sb["settlement_id"]
    pb = sb.get("province_id")
    owner_b = psql(f"SELECT owner_id FROM settlements WHERE id = '{sb_id}'")

    # B frees one gubbe through the real verb, so the take has a worker to send.
    ordinal = psql(f"SELECT max(gubbe_ordinal) FROM settlement_placement WHERE settlement_id = '{sb_id}'")
    if ordinal:
        st, d = http("DELETE", f"{API}/worlds/{w}/provinces/{pb}/placements/{ordinal}", bearer=tok_b)
        if st not in (200, 204):
            sys.exit(f"unplace failed: {st} {d}")

    # B's unit stands guard on the hex (fixture write — the stance is not what is judged).
    unit = psql(f"SELECT id FROM units WHERE owner_id = '{owner_b}' AND world_id = '{w}' "
                f"AND type <> 'nomadic_host' ORDER BY created_at LIMIT 1")
    if not unit:
        sys.exit("B has no unit to stand on the hex")
    psql(f"UPDATE units SET status = 'positioned', stance = 'sentry', q = {x[0]}, r = {x[1]} WHERE id = '{unit}'")

    po = placement_options(w, tok_b, pb)
    hx = next((h for h in po.get("hexes", []) if (h["hex_q"], h["hex_r"]) == x), None)
    print(f"\nworld {w}\nA {name_a} / {PASSWORD}  centre {ca}\nB {name_b} / {PASSWORD}  "
          f"centre {cb}  province {pb}\nhex X {x}  A's gubbar there: {held}")
    print("B placement-options for X:", {k: hx.get(k) for k in
          ("held_by", "takeable", "held_workers", "held_building")} if hx else "MISSING")


if __name__ == "__main__":
    main()
