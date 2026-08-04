#!/usr/bin/env python3
"""Acceptansscenario för inkommande-marsch-notisen (ForeignMarchSighted).

    tools/acceptance.sh up && tools/acceptance.sh reset
    python3 tools/acceptance_incoming_march.py <suffix>

Notisen är den som startar klockan i asynkronitetsgrinden: en Wanax som varit
borta ska vid inloggning se vad som är på väg mot hen OCH när det landar, med
restid kvar att svara på. Scenariot körs från spelarens ingång — registrering,
grundning, marschorder — utan ett enda DB-ingrepp.

Bygger på tools/acceptance_foreign_units.py (samma treWanax-uppställning;
spawnregeln balanserar på hemisfär, så en av de två andra hamnar nära den
första — vem det blir avgörs mätt, inte antaget).

Bevis:
  1. Baslinje: ingen av de tre har någon ForeignMarchSighted.
  2. Angriparen marscherar sista etappen RAKT PÅ försvararens stadshex.
  3. Försvararen får exakt EN notis för den marschen, level 2 (urgent), med
     stadens namn och den tick den landar — medan marschen fortfarande är i
     rörelse, alltså med tid kvar att svara.
  4. Åskådaren (långt bort) och angriparen själv får noll.
  5. Ytterligare två tick senare: fortfarande exakt en. Dedupen håller.
"""
import json, sys, time, urllib.request, urllib.error

BASE = "http://localhost:8097"
API = BASE + "/api/v1"
BLOCKED = {"coastal_sea", "deep_sea", "mountain_limestone", "mountain_red", "river", "fog"}
KIND = "ForeignMarchSighted"
TICK_SECONDS = 6
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


def worlds():
    _, d = http("GET", API + "/worlds")
    return d if isinstance(d, list) else d.get("worlds", [])


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


def settle(who, name):
    """Grunda metropolis där hosten står — försvararen behöver en STAD, inte
    bara en host: level 2 kommer ur att marschens målhex är mottagarens egen
    bosättning, och stadsögat är det som ser hären komma."""
    st, d = api(who, "/founding/settle", "POST", {"name": name})
    assert st in (200, 201), (st, d)
    return d


def city_of(who):
    _, d = api(who, "/settlements")
    ss = d if isinstance(d, list) else (d or {}).get("settlements", [])
    assert ss, f"{who} har ingen stad"
    return ss[0]


def notifs(who):
    st, d = api(who, f"/notifications?kind={KIND}")
    items = d if isinstance(d, list) else (d or {}).get("notifications", [])
    return items or []


def body_of(n):
    b = n.get("body")
    return json.loads(b) if isinstance(b, str) else (b or {})


def knows(who, hex_):
    """Har någon av spelarens män någonsin sett hexen? Marschgrinden kräver det
    av målhexen, så slutmarschen mot staden går inte att lägga förrän hären
    faktiskt fått syn på den."""
    _, tiles = api(who, "/map")
    return any((t["q"], t["r"]) == hex_ and t.get("tier") in ("live", "remembered")
               for t in tiles)


def walk(who, uid, goal, stop_at, deadline):
    """Marscherar etappvis mot goal tills avståndet är <= stop_at. En etapp i
    taget är spelarens riktiga loop — marschgrinden kräver KÄND målhex.

    Varje etapp beställs mot en hex som ligger STRIKT närmare än den nuvarande,
    och samma hex beställs aldrig två gånger: en omordnad marsch får ny
    departs_at och därmed ny march_key, alltså en ny notis hos försvararen.
    Utan spärren mäter scenariot sin egen loop i stället för spelet."""
    here, ordered = None, set()
    while time.time() < deadline:
        u = next((x for x in units(who) if x["id"] == uid), None)
        if u is None:
            return here, 99
        if u["status"] == "marching" or u.get("q") is None:
            time.sleep(2)
            continue
        here = (u["q"], u["r"])
        d = dist(here, goal)
        if d <= stop_at:
            return here, d
        _, tiles = api(who, "/map")
        cand = [t for t in tiles
                if t.get("tier") in ("live", "remembered") and t["terrain"] not in BLOCKED
                and dist((t["q"], t["r"]), goal) < d
                and (t["q"], t["r"]) != here and (t["q"], t["r"]) not in ordered]
        if not cand:
            print(f"    slut på känd mark på {here} ({d} hexar kvar)")
            return here, d
        cand.sort(key=lambda t: dist((t["q"], t["r"]), goal))
        t = cand[0]
        tgt = (t["q"], t["r"])
        st, resp = api(who, f"/units/{uid}/march", "POST",
                       {"target_q": tgt[0], "target_r": tgt[1]})
        if st not in (200, 202):
            print(f"    march avvisad {st}: {resp}")
            return here, d
        ordered.add(tgt)
        print(f"    {here} → {tgt}", flush=True)
        # Vänta på att POSITIONEN faktiskt ändras, inte på att status slutar
        # säga 'marching': enheten står kvar som 'positioned' på sin gamla hex
        # en stund efter att ordern tagits emot, och läser man då av läget
        # beställer man nästa etapp från en föråldrad position — vilket ger en
        # ny marsch, en ny march_key och en notis som mäter scenariots egen
        # loop i stället för spelet.
        moved = time.time() + 60
        while time.time() < moved:
            u = next((x for x in units(who) if x["id"] == uid), None)
            if u and u.get("q") is not None and (u["q"], u["r"]) != here:
                break
            time.sleep(2)
    return here, 99


if __name__ == "__main__":
    sfx = sys.argv[1] if len(sys.argv) > 1 else "x"
    W = worlds()[0]["id"]
    print(f"värld: {W}")

    names = {k: f"Wanax{i}{sfx}" for i, k in enumerate(("A", "B", "C"), 1)}
    for k in ("A", "B", "C"):
        tok[k] = register(names[k])
        print(f"  {k} = {names[k]}  host {host_pos(k)}")

    # Vem som hamnar nära vem avgörs av spawnregeln — mät det, anta det inte.
    hosts = {k: host_pos(k) for k in ("A", "B", "C")}
    others = sorted(("B", "C"), key=lambda k: dist(hosts["A"], hosts[k]))
    DEF, BYS = others[0], others[1]
    print(f"\nangripare A · försvarare {DEF} (avstånd {dist(hosts['A'], hosts[DEF])})"
          f" · åskådare {BYS} (avstånd {dist(hosts['A'], hosts[BYS])})")

    # BARA försvararen grundar. Grundandet drar in hosten och ställer förbanden
    # i garnison utan egen hexposition — angriparen måste därför stå kvar som
    # vandrande folk för att ha en enhet som kan marschera alls, och åskådaren
    # behöver ingen stad för att bevisa att den inte ser något.
    settle(DEF, f"Polis{DEF}{sfx}")
    city = city_of(DEF)
    goal = (city["q"], city["r"]) if "q" in city else None
    if goal is None:
        _, provs = api(DEF, "/provinces")
        pl = provs if isinstance(provs, list) else provs.get("provinces", [])
        p = next(x for x in pl if x.get("settlement_id") == city["id"] or x.get("name") == city["name"])
        goal = (p["q"], p["r"])
    print(f"  {DEF}:s stad {city['name']} på {goal}")

    print(f"\nBASLINJE {KIND}:")
    for k in ("A", "B", "C"):
        n = notifs(k)
        print(f"  {k}: {len(n)}")
        assert not n, f"{k} hade redan en notis vid start: {n}"

    spear = next(u for u in units("A") if u["type"] == "spearman")
    print(f"\nA marscherar {spear['id'][:8]} mot {city['name']} {goal} "
          f"(avstånd {dist((spear['q'], spear['r']), goal)}):")
    end, d = walk("A", spear["id"], goal, 2, time.time() + 600)
    print(f"  framme på {end}, {d} hexar från staden")
    assert knows("A", goal), (
        f"A har aldrig sett stadshexen {goal} (stannade på {end}) — slutmarschen "
        f"kan inte läggas, och utan den finns ingen level 2 att bevisa")
    legs = len(notifs(DEF))
    print(f"  notiser till {DEF} under etappmarschen: {legs}"
          f"   (varje etapp är en ny marsch och alltså en ny notis)")

    # Sista etappen: RAKT på stadshexen. Det är den som ska ge level 2.
    st, resp = api("A", f"/units/{spear['id']}/march", "POST",
                   {"target_q": goal[0], "target_r": goal[1]})
    assert st in (200, 202), f"slutmarschen avvisad: {st} {resp}"
    print(f"\n  slutmarsch mot stadshexen {goal}: HTTP {st}")

    print("\nväntar på skanningen …")
    hit, deadline = None, time.time() + 90
    while time.time() < deadline and hit is None:
        for n in notifs(DEF):
            b = body_of(n)
            if (b.get("target_q"), b.get("target_r")) == goal:
                hit = (n, b)
                break
        if hit is None:
            time.sleep(TICK_SECONDS)

    assert hit, f"{DEF} fick ingen notis om marschen mot sin egen stad inom 90 s"
    n, b = hit
    print(f"\nBEVIS — {DEF} ({names[DEF]}):")
    print(f"  level           {n['level']}   (2 = urgent)")
    print(f"  hotad stad      {b.get('threatens_name')}")
    print(f"  angripare       {b.get('owner')} · {b.get('unit_type')} ×{b.get('size')}"
          f" · stance {b.get('stance')}")
    print(f"  sedd på         ({b.get('q')},{b.get('r')})  → mål ({b.get('target_q')},{b.get('target_r')})")
    print(f"  landar tick     {b.get('arrive_tick')}")
    assert n["level"] == 2, f"level {n['level']}, väntade 2 (marschen går mot mottagarens egen stad)"
    assert b.get("threatens_name") == city["name"], b.get("threatens_name")
    assert b.get("arrive_tick"), "ingen arrive_tick — spelaren kan inte se NÄR det landar"

    # Grindens hela poäng: notisen kom medan hären fortfarande var i rörelse.
    u = next(x for x in units("A") if x["id"] == spear["id"])
    print(f"  hären status    {u['status']}   ← måste vara 'marching': notisen ska "
          f"komma med restid KVAR att svara på")

    print(f"\n  {BYS} ({names[BYS]}): {len(notifs(BYS))} notiser   (långt bort — ska vara 0)")
    print(f"  A ({names['A']}): {len(notifs('A'))} notiser   (sin egen marsch — ska vara 0)")
    assert not notifs(BYS), "åskådaren fick en notis om en marsch den inte kan se"
    assert not notifs("A"), "angriparen varnades om sin egen marsch"

    print("\nväntar två tick till (dedupe) …")
    time.sleep(TICK_SECONDS * 3)
    same = [x for x in notifs(DEF) if (body_of(x).get("target_q"), body_of(x).get("target_r")) == goal]
    print(f"  notiser för samma marsch: {len(same)}  (ska vara 1)")
    assert len(same) == 1, f"dedupen släppte igenom {len(same)} notiser för samma marsch"

    print("\nALLA BEVIS OK")
    json.dump({"world": W, "tokens": tok, "defender": DEF, "city": city["name"]},
              open("scenario_incoming_march.json", "w"))
