#!/usr/bin/env python3
"""BILD fixture: a city with two farms on two grain hexes and a mine on a
silver-deposit hex — megaron_plan_byggnad_pa_hex.md §A2's acceptance fixture.

    tools/acceptance.sh up && tools/acceptance.sh reset
    python3 tools/hexbuildings_fixture.py

Every building is created THROUGH the real game surfaces — join (Nomadic
Host), founding settle, and POST .../build with hex_q/hex_r — never a raw
INSERT into `buildings`. The only direct DB access (`tools/acceptance.sh
psql`) is: (1) READ-ONLY discovery of grain terrains (production_rules) and
which hex has which deposit (map_tiles is not exposed to the client ahead of
founding/building, unlike a plain player who scouts it by walking there —
this script instead re-rolls the Nomadic Host's spawn, see below, and only
reads what that spawn's own founding-preview already reveals to it); (2) a
resource top-up if a fresh capital can't yet afford three builds (the same
"seed enough to afford it" pattern server/api/handlers/*_test.go fixtures
use); (3) fast-forwarding build_queue.complete_at so the tick worker finishes
the builds in seconds of real time instead of minutes — this simulates
WAITING, it does not skip the build: the row was inserted by the real build
endpoint and is still completed by the real tick worker.

Finding a suitable start: join's spawn-tile bias already favours a metal
deposit in catchment (a silver deposit ranks above "no metal", below the
hemisphere's own ore — join.go, megaron_silvergeografin.md), but does not
guarantee one, and never guarantees grain. So this script registers fresh
Nomadic Hosts (join is instant and cheap) and reads each one's own founding
forecast (GET .../colonize-preview at the host's own position — the same
call `keryx founding settle` shows the player before confirming) until it
finds a spawn whose catchment has >= 2 grain-capable hexes and >= 1 silver
deposit hex, then settles THERE. Abandoned hosts are harmless founder_phase
rows in a disposable acceptance world.

Prints: username/password, world id, settlement name, province id, and the
exact (q,r) of every hex-bound building — everything needed to find it in
the running rig without re-deriving anything.
"""
import argparse
import json
import subprocess
import sys
import time
import urllib.error
import urllib.request

BASE = "http://localhost:8097"
API = BASE + "/api/v1"
PASSWORD = "acceptance-pw-123"
MAX_SPAWN_ATTEMPTS = 20


def http(method, url, body=None, bearer=None):
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(url, data=data, method=method)
    req.add_header("Content-Type", "application/json")
    if bearer:
        req.add_header("Authorization", "Bearer " + bearer)
    try:
        with urllib.request.urlopen(req, timeout=25) as r:
            raw = r.read().decode()
            return r.status, (json.loads(raw) if raw.strip() else None)
    except urllib.error.HTTPError as e:
        raw = e.read().decode()
        try:
            return e.code, json.loads(raw)
        except ValueError:
            return e.code, raw[:400]


def psql(sql):
    """tools/acceptance.sh psql <SQL> — read-only discovery / sanctioned
    setup-only writes (never a `buildings` row), same convention as
    acceptance_budets_ben.py's psql() helper."""
    out = subprocess.run(["tools/acceptance.sh", "psql", sql], capture_output=True, text=True)
    if out.returncode != 0:
        sys.exit(f"psql failed: {sql}\n{out.stderr}")
    lines = [l for l in out.stdout.strip().splitlines()
             if not (l.split() and l.split()[0] in ("INSERT", "UPDATE", "DELETE") and l.split()[-1].isdigit())]
    return "\n".join(lines)


def world_id():
    _, d = http("GET", API + "/worlds")
    ws = d if isinstance(d, list) else d.get("worlds", [])
    if not ws:
        sys.exit("no world seeded — run tools/acceptance.sh up/reset first")
    ws.sort(key=lambda w: w.get("created_at", ""), reverse=True)
    return ws[0]["id"]


def register(name):
    st, d = http("POST", API + "/auth/register",
                 {"username": name, "email": name + "@acc.local", "password": PASSWORD})
    if st not in (200, 201):
        sys.exit(f"register {name} failed: {st} {d}")
    return d.get("access_token") or d["token"]


def grain_terrains():
    out = psql("SELECT DISTINCT terrain_type FROM production_rules "
               "WHERE building_type = 'farm' AND terrain_type IS NOT NULL")
    terrains = [l.strip() for l in out.splitlines() if l.strip()]
    if not terrains:
        sys.exit("no grain terrain found in production_rules — fixture cannot proceed")
    return set(terrains)


def founding_status(w, tok):
    st, d = http("GET", f"{API}/worlds/{w}/founding/status", bearer=tok)
    if st != 200:
        sys.exit(f"founding/status failed: {st} {d}")
    return d


def colonize_preview(w, tok, q, r):
    st, d = http("GET", f"{API}/worlds/{w}/colonize-preview?q={q}&r={r}", bearer=tok)
    if st != 200:
        sys.exit(f"colonize-preview failed: {st} {d}")
    return d


def find_settle_site(w, grain_terrains_set):
    """Registers fresh Nomadic Hosts until one's own spawn catchment has >= 2
    grain-capable hexes and >= 1 silver-deposit hex (excluding the centre
    hex itself — production never happens on the settlement's own hex).
    Returns (token, q, r, grain_hexes, silver_hexes)."""
    for attempt in range(1, MAX_SPAWN_ATTEMPTS + 1):
        name = f"hexdemo-{attempt}-{int(time.time())}"
        tok = register(name)
        fp = founding_status(w, tok)
        if not fp.get("active") or fp.get("q") is None:
            continue
        q, r = fp["q"], fp["r"]
        preview = colonize_preview(w, tok, q, r)
        grain_hexes, silver_hexes = [], []
        for hx in preview.get("catchment", []):
            if not hx.get("known") or (hx["q"] == q and hx["r"] == r):
                continue
            if hx.get("terrain") in grain_terrains_set:
                grain_hexes.append((hx["q"], hx["r"]))
            if hx.get("silver_deposit"):
                silver_hexes.append((hx["q"], hx["r"]))
        print(f"  attempt {attempt}: spawn ({q},{r}) — {len(grain_hexes)} grain hex(es), "
              f"{len(silver_hexes)} silver hex(es)")
        if len(grain_hexes) >= 2 and len(silver_hexes) >= 1:
            return tok, q, r, grain_hexes, silver_hexes
    sys.exit(f"no suitable spawn found in {MAX_SPAWN_ATTEMPTS} attempts — "
             "re-run (spawn is randomised) or widen the map (ACC_MAP_WIDTH/HEIGHT)")


def settle(w, tok, name):
    st, d = http("POST", f"{API}/worlds/{w}/founding/settle", {"name": name}, bearer=tok)
    if st not in (200, 201):
        sys.exit(f"founding/settle failed: {st} {d}")
    return d


def get_province(w, tok, province_id):
    st, d = http("GET", f"{API}/worlds/{w}/provinces/{province_id}", bearer=tok)
    if st != 200:
        sys.exit(f"GET province failed: {st} {d}")
    return d


def placement_options(w, tok, province_id):
    st, d = http("GET", f"{API}/worlds/{w}/provinces/{province_id}/placement-options", bearer=tok)
    if st != 200:
        sys.exit(f"placement-options failed: {st} {d}")
    return d


def build(w, tok, province_id, building_type, q=None, r=None):
    body = {"building_type": building_type}
    if q is not None:
        body["hex_q"], body["hex_r"] = q, r
    st, d = http("POST", f"{API}/worlds/{w}/provinces/{province_id}/build", body, bearer=tok)
    if st not in (200, 201):
        sys.exit(f"build {building_type}@({q},{r}) failed: {st} {d}")
    return d


def ensure_afford(settlement_id, need):
    """Tops up timber/stone via settlement_goods if the fresh capital can't
    yet afford three hex-bound builds. Resource seeding, never a `buildings`
    row — same pattern as province_mine_catchment_test.go's fixture."""
    for good, amount in need.items():
        have = float(psql(f"SELECT COALESCE(amount,0) FROM settlement_goods "
                          f"WHERE settlement_id = '{settlement_id}' AND good_key = '{good}'") or "0")
        if have < amount:
            psql(f"INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick) "
                f"VALUES ('{settlement_id}', '{good}', {amount}, 0, {amount}, 0) "
                f"ON CONFLICT (settlement_id, good_key) DO UPDATE SET amount = {amount}")


def wait_for_builds(settlement_id, expect, timeout=120):
    """Fast-forwards build_queue.complete_at to now, then polls the real
    `buildings` row count until the tick worker has finished all `expect`
    hex-bound builds (or timeout). This simulates elapsed real time; the
    completion itself still runs through the normal tick worker."""
    psql(f"UPDATE build_queue SET complete_at = now() - interval '1 minute' "
        f"WHERE settlement_id = '{settlement_id}'")
    deadline = time.time() + timeout
    while time.time() < deadline:
        n = int(psql(f"SELECT count(*) FROM buildings WHERE settlement_id = '{settlement_id}' "
                     f"AND hex_q IS NOT NULL") or "0")
        if n >= expect:
            return
        time.sleep(3)
    sys.exit(f"builds did not complete within {timeout}s — check tools/acceptance.sh logs")


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--name", default="Argyropolis", help="settlement name (silver-themed default)")
    args = ap.parse_args()

    w = world_id()
    print(f"world {w}")

    terrains = grain_terrains()
    print(f"grain terrains: {sorted(terrains)}")

    tok, q, r, grain_hexes, silver_hexes = find_settle_site(w, terrains)
    print(f"settling at ({q},{r}) — {args.name}")
    settled = settle(w, tok, args.name)
    province_id = settled.get("province_id") or settled["province"]["id"]

    prov = get_province(w, tok, province_id)
    sett = prov["settlement"]
    settlement_id = sett["id"]
    auto_farm_hex = None
    for b in sett.get("buildings", []):
        if b["type"] == "farm" and b.get("hex_q") is not None:
            auto_farm_hex = (b["hex_q"], b["hex_r"])
    if auto_farm_hex:
        print(f"  founding gift: free farm already at {auto_farm_hex}")

    opts = placement_options(w, tok, province_id)
    valid_farm = [(h["q"], h["r"]) for h in opts["valid_hexes_for_building"].get("farm", [])]
    valid_mine = [(h["q"], h["r"]) for h in opts["valid_hexes_for_building"].get("mine", [])]
    silver_set = set(silver_hexes)

    if len(valid_farm) < 1:
        sys.exit(f"only {len(valid_farm)} buildable farm hex(es) left after the founding gift — "
                 "re-run (spawn is randomised)")
    mine_candidates = [h for h in valid_mine if h in silver_set]
    if not mine_candidates:
        sys.exit("no server-confirmed buildable silver hex — re-run (spawn is randomised)")

    farm_hex_2 = valid_farm[0]
    mine_hex = mine_candidates[0]
    print(f"  building farm #2 @ {farm_hex_2}, mine @ {mine_hex} (silver)")

    ensure_afford(settlement_id, {"timber": 200, "stone": 200})
    build(w, tok, province_id, "farm", *farm_hex_2)
    build(w, tok, province_id, "mine", *mine_hex)

    expect = (2 if auto_farm_hex else 1) + 1  # two farms (gift + built) + one mine
    wait_for_builds(settlement_id, expect)

    final = get_province(w, tok, province_id)["settlement"]
    print()
    print("── fixture ready ──────────────────────────────────────────")
    print(f"  login      curl {API}/auth/login -d '{{\"username_or_email\":\"...\",\"password\":\"{PASSWORD}\"}}'")
    print(f"             (token already in hand above — reuse it, no need to log in again)")
    print(f"  world      {w}")
    print(f"  settlement {args.name}  (province {province_id})")
    print("  hex-bound buildings:")
    for b in final.get("buildings", []):
        if b.get("hex_q") is not None:
            print(f"    {b['type']:<12} L{b['level']}  @ ({b['hex_q']},{b['hex_r']})")
    print()
    print(f"  keryx: ./keryx build --list; ./keryx city   (after writing this token to a config)")


if __name__ == "__main__":
    main()
