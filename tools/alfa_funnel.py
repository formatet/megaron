#!/usr/bin/env python3
"""alfa_funnel — var i kedjan går varje människa bet? (megaron_plan_alfaanalys punkt 1)

En rad per människa (players.is_ai = false) som har gått med i världen. Cellerna är
SPELDYGNET (tick) då steget nåddes; tomt = aldrig. `N~` = uppskattat (se nedan).
Läser bara — varje fråga körs i BEGIN READ ONLY (se alfa_db.py).

Kolumn -> auktoritativ källa (verifierad mot koden 2026-10-08):

 anslöt                 player_world_records.joined_at (väggklocka)              ~ via tickkalendern
 grundade stad          player_world_records.founded_tick                         exakt
                        (satt av founding settle; NULL tills metropolen finns)
 första placering~      settlement_placement.placed_at (väggklocka), första rad
                        EFTER grundandet som inte är motorns egen: rader inom
                        AUTO_WINDOW_S sekunder efter ett WorldTick-event räknas som
                        tillväxtens auto-placering (kharis/tick.go applyDecay), rader
                        inom samma sekund som grundandet som grundarens startgubbar.
                        INGET event skrivs för placering och borttagna rader syns inte
                        -> bara uppskattning, och en `place -n` utan ny placering syns ej.
 första bygge klart     events.event_type='BuildComplete' (world_tick; stream_id =
                        settlement-id trots stream_type='province'; ägare = settlements.owner_id)
                        exakt. Startfarmen vid grundandet räknas inte (inget event).
 första rekrytering klar events 'TrainComplete' (world_tick; stream_id = settlement), exakt.
                        Poseidons gåvogalär (UnitFormed vid grundandet) räknas inte.
 första marsch          events 'UnitMarchOrdered' (world_tick; stream_id = enhet),
                        ägare via enhet -> ägare (units, UnitFormed.owner_id, TrainComplete), exakt.
                        Omfattar även koloniserings- och utforskningsmarscher.
 första bud~            messengers där kind='diplomatic' (meddelande/handelsförslag),
                        sender_id, sent_at (väggklocka). Order-/recall-löpare räknas ej.
 första handel/gåva/överf. transports.departed_tick, första per owner_id; kind som inte är
                        'damaged_return'/'standing_order_return'. exakt.
 första koloni~         notifications.kind='ColonyFounded' (player_id, created_at) -> tick via kalender.
                        (settlements.founded_from IS NOT NULL bekräftar att kolonin finns.)
 brons i lager          settlement_goods bronze (amount + rate*(current_tick-calc_tick) > 0)
                        i spelarens städer. Ingen tidpunkt finns i DB (lazy-tuple, inget event)
                        -> visas som `nu` om lager finns, annars tomt. EJ MÄTBART som tick.
 elitinfanteri          events 'TrainComplete' med payload.unit_type='elite_infantry', exakt.
 senaste aktivitet~     max av spelarens egna spår: refresh_tokens.created_at (inloggning),
                        messengers.sent_at, transports.created_at, UnitMarchOrdered/TrainComplete-events,
                        icke-motor-placeringar. Visas som tick~ + väggklocka.
 avslag                 antal notifications per kind i REFUSAL_KINDS (asynkrona fel).
 avslag (API)           tabellen refusals (mig 163, TILLFÄLLIG avslagslogg, megaron_plan_avslagslogg):
                        varje 4xx på ett spelarverb, per verb (sista route-segmentet). Saknas tabellen
                        (äldre DB) visas "ej loggat".

Tickkalendern (väggklocka -> tick): WorldTick-eventen (events.event_type='WorldTick') bär både
world_tick och created_at, så en väggklockstid översätts till tickens räknare utan antagande om
kadens. Före äldsta WorldTick-event extrapoleras med medelkadensen; efter det sista används
worlds.current_tick.
"""
import argparse
import bisect
import statistics
import sys
from datetime import datetime

sys.path.insert(0, __import__("os").path.dirname(__import__("os").path.abspath(__file__)))
import alfa_db  # noqa: E402

# Källkonstanter — mutationsprovet ändrar en av dessa.
EV_BUILD = "BuildComplete"
EV_TRAIN = "TrainComplete"
EV_MARCH = "UnitMarchOrdered"
EV_TICK = "WorldTick"
NOTIF_COLONY = "ColonyFounded"
MSG_KIND = "diplomatic"
ELITE = "elite_infantry"
NOT_A_TRANSFER = ("damaged_return", "standing_order_return")
REFUSAL_KINDS = ("OrderFailed", "MarchStalled", "PassageStalled", "PickupTimedOut",
                 "OfferDeclined", "OfferExpired")
AUTO_WINDOW_S = 3.0

COLUMNS = [
    ("joined", "anslöt~"), ("founded", "grundade stad"), ("placed", "första placering~"),
    ("built", "första bygge klart"), ("trained", "första rekrytering klar"),
    ("marched", "första marsch"), ("message", "första bud~"),
    ("transport", "första handel/gåva/överf."), ("colony", "första koloni~"),
    ("bronze", "brons i lager"), ("elite", "elitinfanteri"),
]


def ts(s):
    return datetime.fromisoformat(s)


class Calendar:
    """väggklocka -> tick, byggd på WorldTick-eventen."""

    def __init__(self, pairs, current_tick):
        pairs = sorted(pairs)
        self.times = [p[0] for p in pairs]
        self.ticks = [p[1] for p in pairs]
        self.current = current_tick
        diffs = [(self.times[i + 1] - self.times[i]).total_seconds() / max(1, self.ticks[i + 1] - self.ticks[i])
                 for i in range(min(len(pairs) - 1, 50))]
        self.cadence = statistics.median(diffs) if diffs else None

    def tick(self, dt):
        if not self.times:
            return None
        i = bisect.bisect_right(self.times, dt)
        if i == 0:
            if not self.cadence:
                return None
            back = int((self.times[0] - dt).total_seconds() // self.cadence) + 1
            return max(0, self.ticks[0] - back)
        if i == len(self.times):
            return max(self.ticks[-1], self.current) if dt >= self.times[-1] else self.ticks[-1]
        return self.ticks[i - 1]

    def tick_instants(self):
        return self.times


def load(db, world):
    data = _load(db, world)
    q = lambda sql: db.rows(sql.replace("{W}", world["id"]))
    if data["has_refusals"] and data["has_refusals"][0]["ok"]:
        data["api_refusals"] = q("""SELECT player_id, route, status, code, world_tick FROM refusals
                                    WHERE world_id = '{W}' ORDER BY id""")
    else:
        data["api_refusals"] = None
    return data


def _load(db, world):
    w = world["id"]
    q = lambda sql: db.rows(sql.replace("{W}", w))
    return {
        "players": q("""SELECT p.id, p.username, r.joined_at, r.founded_tick FROM players p
                        JOIN player_world_records r ON r.player_id = p.id AND r.world_id = '{W}'
                        WHERE p.is_ai = false ORDER BY r.joined_at"""),
        "unjoined": q("""SELECT count(*) AS n FROM players p WHERE p.is_ai = false AND NOT EXISTS
                         (SELECT 1 FROM player_world_records r WHERE r.player_id = p.id AND r.world_id = '{W}')"""),
        "settlements": q("SELECT id, owner_id, founded_from FROM settlements WHERE world_id = '{W}'"),
        "events": q("""SELECT event_type, world_tick, stream_id, created_at, payload FROM events
                       WHERE world_id = '{W}' AND event_type IN ('%s','%s','%s','UnitFormed')""" % (EV_BUILD, EV_TRAIN, EV_MARCH)),
        "ticks": q("SELECT world_tick, created_at FROM events WHERE world_id = '{W}' AND event_type = '%s'" % EV_TICK),
        "units": q("SELECT id, owner_id FROM units WHERE world_id = '{W}'"),
        "placements": q("""SELECT s.owner_id, sp.placed_at, sp.settlement_id FROM settlement_placement sp
                           JOIN settlements s ON s.id = sp.settlement_id WHERE s.world_id = '{W}'"""),
        "messengers": q("SELECT sender_id, sent_at FROM messengers WHERE world_id = '{W}' AND kind = '%s'" % MSG_KIND),
        "transports": q("""SELECT owner_id, departed_tick, created_at FROM transports WHERE world_id = '{W}'
                           AND kind NOT IN ('%s','%s')""" % NOT_A_TRANSFER),
        "notifs": q("""SELECT player_id, kind, created_at FROM notifications WHERE world_id = '{W}'
                       AND kind IN ('%s', %s)""" % (NOTIF_COLONY, ",".join("'%s'" % k for k in REFUSAL_KINDS))),
        "bronze": q("""SELECT s.owner_id, sg.amount + sg.rate * greatest(0, w.current_tick - sg.calc_tick) AS stock
                       FROM settlement_goods sg JOIN settlements s ON s.id = sg.settlement_id
                       JOIN worlds w ON w.id = s.world_id
                       WHERE s.world_id = '{W}' AND sg.good_key = 'bronze'"""),
        "logins": q("SELECT player_id, max(created_at) AS last FROM refresh_tokens GROUP BY player_id"),
        "has_refusals": q("SELECT to_regclass('public.refusals') IS NOT NULL AS ok"),
    }


def build(data, world):
    cal = Calendar([(ts(t["created_at"]), t["world_tick"]) for t in data["ticks"]], world["current_tick"])
    owner_of_settlement = {s["id"]: s["owner_id"] for s in data["settlements"]}
    # enhet -> ägare: units, UnitFormed.owner_id, TrainComplete (stream_id = settlement)
    unit_owner = {u["id"]: u["owner_id"] for u in data["units"]}
    for e in data["events"]:
        p = e["payload"]
        if e["event_type"] == "UnitFormed" and p.get("owner_id"):
            unit_owner.setdefault(p["unit_id"], p["owner_id"])
        elif e["event_type"] == EV_TRAIN and p.get("unit_id"):
            unit_owner.setdefault(p["unit_id"], owner_of_settlement.get(e["stream_id"]))

    cells = {p["id"]: {} for p in data["players"]}
    est = {k: set() for k in cells}
    last_wall = {}

    def put(pid, col, tick, estimated=False):
        if pid not in cells or tick is None:
            return
        cur = cells[pid].get(col)
        if cur is None or tick < cur:
            cells[pid][col] = tick
            (est[pid].add if estimated else est[pid].discard)(col)

    def seen(pid, dt):
        if pid in cells and dt is not None and (pid not in last_wall or dt > last_wall[pid]):
            last_wall[pid] = dt

    for p in data["players"]:
        put(p["id"], "joined", cal.tick(ts(p["joined_at"])), True)
        if p["founded_tick"] is not None:
            put(p["id"], "founded", p["founded_tick"])
    for e in data["events"]:
        t, p, et = e["world_tick"], e["payload"], e["event_type"]
        if et == EV_BUILD:
            o = owner_of_settlement.get(e["stream_id"]); put(o, "built", t)
        elif et == EV_TRAIN:
            o = owner_of_settlement.get(e["stream_id"]); put(o, "trained", t); seen(o, ts(e["created_at"]))
            if p.get("unit_type") == ELITE:
                put(o, "elite", t)
        elif et == EV_MARCH:
            o = unit_owner.get(e["stream_id"]); put(o, "marched", t); seen(o, ts(e["created_at"]))
    for m in data["messengers"]:
        d = ts(m["sent_at"]); put(m["sender_id"], "message", cal.tick(d), True); seen(m["sender_id"], d)
    for t in data["transports"]:
        put(t["owner_id"], "transport", t["departed_tick"] if t["departed_tick"] is not None else cal.tick(ts(t["created_at"])),
            t["departed_tick"] is None)
        seen(t["owner_id"], ts(t["created_at"]))
    refusals = {k: {} for k in cells}
    for n in data["notifs"]:
        if n["kind"] == NOTIF_COLONY:
            put(n["player_id"], "colony", cal.tick(ts(n["created_at"])), True)
        elif n["player_id"] in refusals:
            refusals[n["player_id"]][n["kind"]] = refusals[n["player_id"]].get(n["kind"], 0) + 1
    for b in data["bronze"]:
        if b["stock"] and b["stock"] > 0 and b["owner_id"] in cells:
            cells[b["owner_id"]]["bronze"] = "nu"
    for l in data["logins"]:
        seen(l["player_id"], ts(l["last"]))

    # placeringar: hoppa över grundarens startbatch och motorns tillväxt-placering
    instants = cal.tick_instants()
    founding = {}  # settlement -> tidigaste placed_at (startbatchen)
    for pl in data["placements"]:
        d = ts(pl["placed_at"])
        if pl["settlement_id"] not in founding or d < founding[pl["settlement_id"]]:
            founding[pl["settlement_id"]] = d
    for pl in data["placements"]:
        d = ts(pl["placed_at"])
        if (d - founding[pl["settlement_id"]]).total_seconds() <= AUTO_WINDOW_S:
            continue
        i = bisect.bisect_right(instants, d)
        if i and (d - instants[i - 1]).total_seconds() <= AUTO_WINDOW_S:
            continue
        put(pl["owner_id"], "placed", cal.tick(d), True); seen(pl["owner_id"], d)

    rows = []
    for p in data["players"]:
        pid = p["id"]
        r = [p["username"]]
        for key, _ in COLUMNS:
            v = cells[pid].get(key)
            r.append("" if v is None else (str(v) + "~" if key in est[pid] else str(v)))
        lw = last_wall.get(pid)
        r.append("" if lw is None else "%s~ (%s)" % (cal.tick(lw), lw.astimezone().strftime("%m-%d %H:%M")))
        rf = refusals[pid]
        r.append(", ".join("%s:%d" % kv for kv in sorted(rf.items())))
        if data["api_refusals"] is None:
            r.append("ej loggat")
        else:
            per = {}
            for a in data["api_refusals"]:
                if a["player_id"] == pid:
                    verb = route_verb(a["route"])
                    per[verb] = per.get(verb, 0) + 1
            r.append(", ".join("%s:%d" % kv for kv in sorted(per.items(), key=lambda kv: -kv[1])))
        rows.append(r)
    return rows, cal


def route_verb(route):
    """'/api/v1/worlds/{worldID}/provinces/{provinceID}/build' -> 'build'; id-segment hoppas över."""
    parts = [p for p in route.strip("/").split("/") if p and not p.startswith("{")]
    return parts[-1] if parts else route


def render(world, data, rows, extra_note=""):
    header = ["spelare"] + [h for _, h in COLUMNS] + ["senaste aktivitet", "avslag (notiser)", "avslag (API)"]
    out = ["# Alfa-funnel — värld %s (%s), tick %s, %s\n" % (world["name"], world["id"][:8], world["current_tick"], world["state"]),
           alfa_db.md_table(header, rows) if rows else "_inga människor har gått med i världen_", ""]
    out.append("Celler = speldygn (tick). `N~` = uppskattat ur väggklocka via WorldTick-kalendern; tomt = aldrig. "
               "`nu` (brons) = lager finns nu, tidpunkt ej mätbar i DB.")
    if data["api_refusals"]:
        top = {}
        for a in data["api_refusals"]:
            k = (route_verb(a["route"]), a["status"], a["code"])
            top[k] = top.get(k, 0) + 1
        out.append("\n## Vanligaste API-avslagen (alla spelare)\n")
        out.append(alfa_db.md_table(["verb", "status", "kod/text", "antal"],
                                    [[v, st, c, n] for (v, st, c), n in sorted(top.items(), key=lambda kv: -kv[1])[:20]]))
        out.append("")
    if data["api_refusals"] is None:
        out.append("Ej mätbart i DB: HTTP-avslag från API:t — tabellen refusals saknas (före mig 163).")
    out.append("Ej mätbart i DB: borttagna placeringar; när bronset först uppstod.")
    n = data["unjoined"][0]["n"] if data["unjoined"] else 0
    out.append("Registrerade människor som aldrig gått med i den här världen: %s." % n)
    return "\n".join(out)


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    alfa_db.add_args(ap)
    args = ap.parse_args(argv)
    db = alfa_db.connect(args)
    world = alfa_db.pick_world(db, args.world)
    data = load(db, world)
    rows, _ = build(data, world)
    print(render(world, data, rows))


if __name__ == "__main__":
    main()
