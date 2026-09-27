#!/usr/bin/env python3
"""Stillbildsfixtur för budets tre ben (megaron_plan_budets_tre_ben.md, slice 3c).

    tools/acceptance.sh up && tools/acceptance.sh reset
    python3 tools/acceptance_budets_ben.py <suffix>

Detta är en BILD-fixtur, inte ett flödestest (planens egen skillnad): en
Wanax registreras och grundar sin stad genom spelarens riktiga ingång (real
HTTP), men de fem benen — och en sjätte, väntande+stalled, variant — skrivs
in direkt i DB via `tools/acceptance.sh psql`, exakt som planen sanktionerar
("direkta DB-skrivningar ... är tillåtna"). Geografin (en andra hamnstad,
en frikopplad avstigningshex) är SQL, inte en riktig sjöresa — poängen är att
bevisa hur kartan RITAR de fem lägena, inte att reproducera hela
bordningsmekaniken.

Tidsfönstren ligger timmar framåt (mätt i world-ticks à TICK_SECONDS, läst ur
riggen snarare än antaget) så lägena håller stilla medan Timothy tittar:
inget av dem beror på någon riktig avgång, bordning eller notis-scan som
skulle kunna flytta det vidare under en tittning.

Skriver ut användarnamn, lösenord, URL och en hex att centrera på per läge.
"""
import json
import subprocess
import sys
import urllib.error
import urllib.request

BASE = "http://localhost:8097"
API = BASE + "/api/v1"
PASSWORD = "acceptance-pw-123"
BLOCKED = {"coastal_sea", "deep_sea", "mountain_limestone", "mountain_red", "river", "fog"}


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
        return e.code, e.read().decode()[:300]


def psql(sql):
    """tools/acceptance.sh psql <SQL> — the fixture's own sanctioned direct
    DB write/read (planens BILD-rigg-stycke). Returns raw -tAc stdout
    (unaligned, '|'-separated columns, one row per line) with an
    INSERT/UPDATE/DELETE completion tag (psql prints it even under -t when
    the statement carries RETURNING) stripped off."""
    out = subprocess.run(["tools/acceptance.sh", "psql", sql], capture_output=True, text=True)
    if out.returncode != 0:
        sys.exit(f"psql misslyckades: {sql}\n{out.stderr}")
    lines = [l for l in out.stdout.strip().splitlines()
             if not (l.split() and l.split()[0] in ("INSERT", "UPDATE", "DELETE") and l.split()[-1].isdigit())]
    return "\n".join(lines)


def psql_row(sql):
    row = psql(sql)
    assert row, f"tom rad från: {sql}"
    return row.split("|")


def world_id():
    _, d = http("GET", API + "/worlds")
    ws = d if isinstance(d, list) else d.get("worlds", [])
    ws.sort(key=lambda w: w.get("created_at", ""), reverse=True)
    return ws[0]["id"]


def register(name):
    st, d = http("POST", API + "/auth/register",
                 {"username": name, "email": name + "@acc.local", "password": PASSWORD})
    assert st in (200, 201), (st, d)
    return d.get("access_token") or d["token"]


def api(w, tok, path, method="GET", body=None):
    return http(method, f"{API}/worlds/{w}{path}", body, tok)


def tick_seconds():
    prov = subprocess.run(["tools/acceptance.sh", "provenance"], capture_output=True, text=True, check=True).stdout
    for part in prov.split("·"):
        part = part.strip()
        if part.startswith("TICK_SECONDS="):
            return int(part.split("=", 1)[1])
    return 60


def land_hex_near(world, q, r, min_d, max_d, exclude_provinces, exclude_hexes=()):
    """One free-ish land hex at hex distance [min_d, max_d] from (q,r) — used
    for the port and the disembark point. `exclude_provinces=True` also
    requires no existing province there (needed for a real settlement; not
    needed for a bare waypoint hex). `exclude_hexes` rules out specific
    (q,r) pairs already claimed by an earlier fixture in THIS script (two
    bands measured from different centres can still converge on the same
    nearest tile). The acceptance world's map is small (30x20 default) and a
    capital can spawn near its edge, so a band with nothing in it just
    widens outward rather than failing outright."""
    extra = (
        f"AND NOT EXISTS (SELECT 1 FROM provinces p WHERE p.world_id = mt.world_id "
        f"AND p.map_q = mt.q AND p.map_r = mt.r)"
        if exclude_provinces else ""
    )
    for hq, hr in exclude_hexes:
        extra += f" AND NOT (mt.q = {hq} AND mt.r = {hr})"
    blocked = ",".join(f"'{t}'" for t in BLOCKED)
    for widen in range(6):
        lo, hi = min_d, max_d + widen * 3
        sql = (
            f"SELECT mt.q, mt.r FROM map_tiles mt "
            f"WHERE mt.world_id = '{world}' AND mt.terrain NOT IN ({blocked}) {extra} "
            f"AND (abs(mt.q - ({q})) + abs((mt.q - ({q})) + (mt.r - ({r}))) + abs(mt.r - ({r}))) BETWEEN {lo} AND {hi} "
            f"ORDER BY (abs(mt.q - ({q})) + abs((mt.q - ({q})) + (mt.r - ({r}))) + abs(mt.r - ({r}))) ASC LIMIT 1"
        )
        row = psql(sql)
        if row:
            qq, rr = row.split("|")
            return int(qq), int(rr)
    sys.exit(f"ingen ledig landhex hittades {min_d}..{max_d}(+widen) hexar från ({q},{r})")


def make_settlement(world, owner_id, name, q, r, coastal):
    prov_id = psql_row(
        f"INSERT INTO provinces (world_id, map_q, map_r, terrain_type, coastal) "
        f"VALUES ('{world}', {q}, {r}, 'plains', {str(coastal).lower()}) RETURNING id"
    )[0]
    sett_id = psql_row(
        f"INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, state, population) "
        f"VALUES ('{world}', '{prov_id}', '{name}', 'achaean', '{owner_id}', 'colony', false, 'active', 3000) RETURNING id"
    )[0]
    return sett_id


def make_ship(world, owner_id, q, r, target_q, target_r, depart_tick, arrive_tick, name):
    row = psql_row(
        f"INSERT INTO units (world_id, owner_id, type, category, size, crew, status, q, r, "
        f"target_q, target_r, depart_tick, arrive_tick, departs_at, arrives_at, name) "
        f"VALUES ('{world}', '{owner_id}', 'merchantman', 'naval', 1, 10, 'marching', {q}, {r}, "
        f"{target_q}, {target_r}, {depart_tick}, {arrive_tick}, now(), now() + interval '3 hours', '{name}') "
        f"RETURNING id"
    )
    return row[0]


def insert_messenger(world, sender_id, origin_id, dest_id, cols):
    fields = ["world_id", "sender_id", "origin_id", "destination_id", "message_text", "status", "kind", "hex_q", "hex_r"]
    values = [f"'{world}'", f"'{sender_id}'", f"'{origin_id}'", f"'{dest_id}'", "'hi'", "'outbound'", "'message'", "0", "0"]
    for k, v in cols.items():
        fields.append(k)
        values.append(v)
    sql = f"INSERT INTO messengers ({', '.join(fields)}) VALUES ({', '.join(values)}) RETURNING id"
    return psql_row(sql)[0]


if __name__ == "__main__":
    sfx = sys.argv[1]
    W = world_id()
    name = f"Budben{sfx}"
    print(f"värld: {W}")

    tok = register(name)
    st, d = http("POST", f"{API}/worlds/{W}/join", {}, tok)
    assert st in (200, 201, 202), (st, d)

    # The world stays 'forming' (orders refused, including founding) until
    # POLEIA_WORLD_START_WANAXES (default 4) have joined — three silent
    # fillers, never touched again, just to start the clock.
    for i in range(3):
        ftok = register(f"{name}Filler{i}")
        st, d = http("POST", f"{API}/worlds/{W}/join", {}, ftok)
        assert st in (200, 201, 202), (st, d)

    st, d = api(W, tok, "/units")
    host = next(u for u in (d or {}).get("units", []) if u["type"] == "nomadic_host")
    st, d = api(W, tok, "/founding/settle", "POST", {"name": f"{name}stad"})
    assert st in (200, 201), (st, d)
    print(f"  huvudstad grundad vid ({host['q']},{host['r']})")

    player_id = psql_row(f"SELECT id FROM players WHERE username = '{name}'")[0]
    origin_id, origin_q, origin_r = psql_row(
        f"SELECT s.id, p.map_q, p.map_r FROM settlements s JOIN provinces p ON p.id = s.province_id "
        f"WHERE s.owner_id = '{player_id}' AND s.is_capital LIMIT 1"
    )
    origin_q, origin_r = int(origin_q), int(origin_r)

    TICK_S = tick_seconds()
    current_tick = int(psql_row(f"SELECT current_tick FROM worlds WHERE id = '{W}'")[0])

    def ticks(hours):
        return round(hours * 3600 / TICK_S)

    # Port (R1's "kuststad") a few hexes out, and a disembark waypoint further
    # still, both on real, existing map_tiles land so the map renders them
    # honestly — no new terrain, only new settlement/messenger/unit rows.
    # `claimed` accumulates every hex picked so far: two bands measured from
    # different centres (capital vs. port) can otherwise converge on the same
    # nearest tile and silently stack two legs' runners on one hex.
    claimed = [(origin_q, origin_r)]

    def pick(cq, cr, lo, hi, exclude_provinces):
        q, r = land_hex_near(W, cq, cr, lo, hi, exclude_provinces=exclude_provinces, exclude_hexes=claimed)
        claimed.append((q, r))
        return q, r

    port_q, port_r = pick(origin_q, origin_r, 4, 7, exclude_provinces=True)
    port_id = make_settlement(W, player_id, f"{name}hamn", port_q, port_r, coastal=True)
    print(f"  hamn skapad vid ({port_q},{port_r})")
    disembark_q, disembark_r = pick(port_q, port_r, 5, 6, exclude_provinces=False)
    print(f"  avstigningspunkt ({disembark_q},{disembark_r})")

    # Three of the five legs (waiting, waiting+stalled, sealed) all stand
    # AT their own port — sharing one settlement would stack all three
    # runners on the same hex and make the screenshots unreadable. Each gets
    # its own small port a short, distinct hop from the capital instead, at
    # bands spread out from each other so none collide by coincidence.
    port2_q, port2_r = pick(origin_q, origin_r, 2, 3, exclude_provinces=True)
    port2_id = make_settlement(W, player_id, f"{name}hamn2", port2_q, port2_r, coastal=True)
    port3_q, port3_r = pick(origin_q, origin_r, 6, 7, exclude_provinces=True)
    port3_id = make_settlement(W, player_id, f"{name}hamn3", port3_q, port3_r, coastal=True)
    # The aboard ship's own frozen hex (where the carrier — and so the
    # runner riding it — is actually drawn) also gets its own spot, distinct
    # from every port and from the disembark point, so no two legs overlap.
    aboard_q, aboard_r = pick(origin_q, origin_r, 10, 12, exclude_provinces=False)

    hexes = {}

    # 1. to_port — walking to its own port, hours still to go.
    insert_messenger(W, player_id, origin_id, port_id, {
        "sent_at": "now() - interval '20 minutes'",
        "arrives_at": "now() + interval '3 hours'",
        "passage_status": "'awaiting_passage'",
        "passage_port_id": f"'{port_id}'",
        "passage_since_tick": str(current_tick + ticks(3) - 1),
    })
    hexes["to_port"] = (origin_q, origin_r)

    # 2. waiting — already at its own (second) port, no carrier yet, no stall notice.
    insert_messenger(W, player_id, origin_id, port2_id, {
        "sent_at": "now() - interval '4 hours'",
        "arrives_at": "now() - interval '3 hours'",
        "passage_status": "'awaiting_passage'",
        "passage_port_id": f"'{port2_id}'",
        "passage_since_tick": str(current_tick - ticks(3)),
    })
    hexes["waiting"] = (port2_q, port2_r)

    # 2b. waiting + stalled — same, but a PassageStalled dispatch already
    #     fired, standing at the FIRST port so it does not overlap #2.
    insert_messenger(W, player_id, origin_id, port_id, {
        "sent_at": "now() - interval '4 hours'",
        "arrives_at": "now() - interval '3 hours'",
        "passage_status": "'awaiting_passage'",
        "passage_port_id": f"'{port_id}'",
        "passage_since_tick": str(current_tick - ticks(3)),
        "passage_stalled_notified_tick": str(current_tick - ticks(1)),
    })
    hexes["waiting_stalled"] = (port_q, port_r)

    # 3. aboard — mid-crossing, on a real ship that is actually 'marching'
    #    and will show up in /units for this same Wanax. The ship's own
    #    frozen hex (aboard_q/r) is its own spot, distinct from every port.
    ship_depart_tick = current_tick - ticks(0.5)
    ship_arrive_tick = current_tick + ticks(3)
    ship_id = make_ship(W, player_id, aboard_q, aboard_r, disembark_q, disembark_r,
                        ship_depart_tick, ship_arrive_tick, "Budben Carrier")
    insert_messenger(W, player_id, origin_id, port_id, {
        "sent_at": "now() - interval '4 hours'",
        "arrives_at": "now() + interval '4 hours'",
        "passage_status": "'aboard'",
        "passage_port_id": f"'{port_id}'",
        "carrier_unit_id": f"'{ship_id}'",
        "carrier_name": "'Budben Carrier'",
        "disembark_q": str(disembark_q),
        "disembark_r": str(disembark_r),
        "boarded_at": "now() - interval '30 minutes'",
        "disembark_at": "now() + interval '3 hours'",
    })
    hexes["aboard"] = (aboard_q, aboard_r)

    # 4. ashore — disembarked, walking the last land leg to the true target
    #    (the port settlement itself — see the script's own doc comment: this
    #    is a stillbildsfixture, not a real return trip).
    ashore_ship_id = psql_row(
        f"INSERT INTO units (world_id, owner_id, type, category, size, crew, status, settlement_id, name) "
        f"VALUES ('{W}', '{player_id}', 'merchantman', 'naval', 1, 10, 'garrison', '{port_id}', 'Budben Ashore Carrier') "
        f"RETURNING id"
    )[0]
    insert_messenger(W, player_id, origin_id, port_id, {
        "sent_at": "now() - interval '5 hours'",
        "arrives_at": "now() + interval '3 hours'",
        "passage_status": "'aboard'",
        "passage_port_id": f"'{port_id}'",
        "carrier_unit_id": f"'{ashore_ship_id}'",
        "carrier_name": "'Budben Ashore Carrier'",
        "disembark_q": str(disembark_q),
        "disembark_r": str(disembark_r),
        "boarded_at": "now() - interval '2 hours'",
        "disembark_at": "now() - interval '1 hours'",
    })
    hexes["ashore"] = (disembark_q, disembark_r)

    # 5. sealed — carrier lost, real position unknown, never promotes during
    #    the viewing session (passage_lost_until_tick far in the future).
    #    Its own (third) port, distinct from every other leg's hex.
    insert_messenger(W, player_id, origin_id, port3_id, {
        "sent_at": "now() - interval '5 hours'",
        "arrives_at": "now() + interval '3 hours'",
        "passage_status": "'returning_sealed'",
        "passage_port_id": f"'{port3_id}'",
        "passage_lost_until_tick": str(current_tick + 100000),
    })
    hexes["sealed"] = (port3_q, port3_r)

    print("\n  login")
    print(f"  användarnamn  {name}")
    print(f"  lösenord      {PASSWORD}")
    print(f"  URL           {BASE}/world/{W}/map")
    print("\n  hexlista (centrera kartan här per läge):")
    for leg, (q, r) in hexes.items():
        print(f"    {leg:16} ({q},{r})")

    json.dump({"world": W, "username": name, "password": PASSWORD, "hexes": hexes},
              open("budets_ben_fixture.json", "w"), indent=2)
