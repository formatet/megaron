#!/usr/bin/env python3
"""Acceptansscenario för främmande enheter — hela flödet från spelarens ingång.

    python3 scenario.py <suffix>

1. Registrerar tre Wanaxes och ansluter dem (spawnregeln balanserar hemisfärer,
   så nr 3 hamnar i samma halva som nr 1 — det är enda sättet att få två
   spelare på gångavstånd utan DB-ingrepp).
2. Baslinje: /foreign-units ska vara tom för alla tre.
3. Wanax1 marscherar sin spjutbärare mot Wanax3:s host i etapper (marschgrinden
   kräver KÄND målhex, så en etapp i taget är spelarens riktiga loop).
4. Bevis: Wanax1 ser Wanax3:s enheter, Wanax2 (långt borta) ser fortfarande
   ingenting, och världs-id:t är detsamma i början och slutet.
"""
import json, sys, time, urllib.request, urllib.error

BASE = "http://localhost:8097"
API = BASE + "/api/v1"
BLOCKED = {"coastal_sea", "deep_sea", "mountain_limestone", "mountain_red", "river", "fog"}
tok = {}


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


def world_id():
    _, d = http("GET", API + "/worlds")
    ws = d if isinstance(d, list) else d.get("worlds", [])
    return ws[0]["id"]


def api(who, path, method="GET", body=None):
    return http(method, f"{API}/worlds/{W}{path}", body, tok[who])


def dist(a, b):
    dq, dr = a[0] - b[0], a[1] - b[1]
    return (abs(dq) + abs(dq + dr) + abs(dr)) // 2


def register(name):
    st, d = http("POST", API + "/auth/register",
                 {"username": name, "email": name + "@acc.local", "password": "acceptance-pw-123"})
    assert st in (200, 201), (st, d)
    t = d.get("access_token") or d["token"]
    st, d = http("POST", f"{API}/worlds/{W}/join", {}, t)
    assert st in (200, 201, 202), (st, d)
    return t


def units(who):
    _, d = api(who, "/units")
    return (d or {}).get("units", [])


def host_pos(who):
    return next(((u["q"], u["r"]) for u in units(who) if u["type"] == "nomadic_host"), None)


def walk(who, uid, goal, stop_at, deadline):
    while time.time() < deadline:
        u = next(x for x in units(who) if x["id"] == uid)
        if u["status"] == "marching":
            time.sleep(3)
            continue
        here = (u["q"], u["r"])
        d = dist(here, goal)
        if d <= stop_at:
            return here, d
        _, tiles = api(who, "/map")
        cand = [t for t in tiles
                if t.get("tier") in ("live", "remembered") and t["terrain"] not in BLOCKED
                and dist((t["q"], t["r"]), goal) < d]
        if not cand:
            return here, d
        cand.sort(key=lambda t: dist((t["q"], t["r"]), goal))
        t = cand[0]
        st, resp = api(who, f"/units/{uid}/march", "POST",
                       {"target_q": t["q"], "target_r": t["r"]})
        if st not in (200, 202):
            print(f"    march avvisad {st}: {resp}")
            return here, d
        print(f"    → ({t['q']},{t['r']})", flush=True)
        time.sleep(3)
    return here, d


if __name__ == "__main__":
    sfx = sys.argv[1]
    W = world_id()
    print(f"värld vid start: {W}")
    names = [f"Wanax1{sfx}", f"Wanax2{sfx}", f"Wanax3{sfx}"]
    for k, n in zip(("T1", "T2", "T3"), names):
        tok[k] = register(n)
        print(f"  {k} = {n}  host {host_pos(k)}")
    json.dump(tok, open("tokens.json", "w"))

    print("\nBASLINJE /foreign-units:")
    for k in ("T1", "T2", "T3"):
        st, d = api(k, "/foreign-units")
        print(f"  {k}: HTTP {st}  {len(d)} enheter")
        assert d == [], f"{k} såg något redan vid start: {d}"

    goal = host_pos("T3")
    spear = next(u for u in units("T1") if u["type"] == "spearman")
    print(f"\nWanax1 marscherar {spear['id'][:8]} mot Wanax3:s host {goal} "
          f"(avstånd {dist((spear['q'], spear['r']), goal)}):")
    end, d = walk("T1", spear["id"], goal, 2, time.time() + 480)
    print(f"  slutposition {end}, {d} hexar från målet")

    print("\nBEVIS /foreign-units:")
    for k in ("T1", "T2", "T3"):
        st, res = api(k, "/foreign-units")
        print(f"  {k}: HTTP {st}  {len(res)} enheter")
        for u in res:
            print(f"     {u['owner']:12} {u['type']:14} ×{u['size']:<4} {u['status']:11} "
                  f"({u['q']},{u['r']})  stance={u.get('stance', '-')}")
    print(f"\nvärld vid slut:  {world_id()}  (samma som start: {world_id() == W})")
    json.dump({"world": W, "tokens": tok}, open("scenario_state.json", "w"))
