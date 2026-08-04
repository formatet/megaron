#!/usr/bin/env python3
"""Mäter upptäcktsfönstret för en egen marsch — beviset bakom "enheter teleporterar".

    python3 tools/acceptance_march_visibility.py <suffix>

Buggen (fixad i `webb: klienten upptacker sin egen marsch`) var att
`ui/marchctx.js` aldrig hämtade om `/units` efter en marschorder, och att
`render/map.js` 3-sekunderspoll är **gatad** på att `State.unitsData` redan
innehåller en marscherande enhet. Snabbpollen kunde alltså inte starta sig själv,
och kvar blev 30-sekunderspollen.

Det här scriptet mäter den enda siffra som avgör om det syns eller inte:
**hur många realsekunder en marsch varar**. Är marschen kortare än
upptäcktslatensen hinner klienten aldrig rita den ett enda frame — enheten står
i staden och står sedan framme. Det ÄR teleportering, sett från spelarens stol.

Scriptet bevisar inte JS-fixen (det kräver en webbläsare) — det bevisar
**mekanismen och storleksordningen**, och det är den delen som annars bara är
ett påstående. Ögonkollen står i megaron_todo.md.
"""
import json, sys, time, urllib.request, urllib.error

BASE = "http://localhost:8097"
API = BASE + "/api/v1"
BLOCKED = {"coastal_sea", "deep_sea", "mountain_limestone", "mountain_red", "river", "fog"}
SLOW_POLL_SECONDS = 30  # render/map.js: setInterval(..., 30000)
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


def world():
    _, d = http("GET", API + "/worlds")
    ws = d if isinstance(d, list) else d.get("worlds", [])
    return ws[0]


def api(path, method="GET", body=None):
    return http(method, f"{API}/worlds/{W}{path}", body, tok["T1"])


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


def units():
    _, d = api("/units")
    return (d or {}).get("units", [])


if __name__ == "__main__":
    sfx = sys.argv[1] if len(sys.argv) > 1 else "mv"
    w = world()
    W = w["id"]
    tick_s = w.get("tick_seconds")
    print(f"värld {W}  tick_seconds={tick_s}")

    tok["T1"] = register(f"Wanax1{sfx}")
    spear = next(u for u in units() if u["type"] == "spearman")
    here = (spear["q"], spear["r"])
    print(f"spjutbärare {spear['id'][:8]} i {here}, status {spear['status']}")

    # Marschgrinden kräver KÄND målhex — välj den kända, framkomliga hex som
    # ligger längst bort, så marschen blir så lång den kan bli i ett steg.
    _, tiles = api("/map")
    cand = [t for t in tiles
            if t.get("tier") in ("live", "remembered") and t["terrain"] not in BLOCKED]
    cand.sort(key=lambda t: dist((t["q"], t["r"]), here), reverse=True)
    target = cand[0]
    hexes = dist((target["q"], target["r"]), here)
    print(f"mål ({target['q']},{target['r']}) — {hexes} hexar, terräng {target['terrain']}")

    st, resp = api(f"/units/{spear['id']}/march", "POST",
                   {"target_q": target["q"], "target_r": target["r"]})
    assert st in (200, 202), (st, resp)
    print(f"marschorder: HTTP {st}")

    u = next(x for x in units() if x["id"] == spear["id"])
    assert u["status"] == "marching", f"enheten marscherar inte: {u['status']}"
    dep = u["departs_at"].replace("Z", "+00:00")
    arr = u["arrives_at"].replace("Z", "+00:00")
    from datetime import datetime
    secs = (datetime.fromisoformat(arr) - datetime.fromisoformat(dep)).total_seconds()

    print(f"\nMARSCHENS LÄNGD: {secs:.0f} realsekunder ({hexes} hexar)")
    print(f"KLIENTENS LÅNGSAMMA POLL: {SLOW_POLL_SECONDS} s")
    if secs < SLOW_POLL_SECONDS:
        print(f"⇒ HELA marschen ryms inuti ett pollintervall. Utan att marschordern\n"
              f"  själv hämtar om /units kan klienten aldrig rita ett enda frame av\n"
              f"  den — enheten teleporterar. Det är buggen, mätt.")
    else:
        print(f"⇒ Marschen är längre än pollintervallet: den långsamma pollen hinner\n"
              f"  se den {secs / SLOW_POLL_SECONDS:.1f} gånger. Teleporteringen syns då\n"
              f"  bara som ett hopp i början, inte som ett försvinnande.")
    print(f"\nAntal sampel klienten HADE fått utan fixen: {int(secs // SLOW_POLL_SECONDS)}")
    print(f"Antal sampel klienten får MED fixen (3 s snabbpoll): {int(secs // 3)}")
