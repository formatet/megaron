#!/usr/bin/env python3
"""alfa_economy — ekonomin per stad och per Wanax (megaron_plan_alfaanalys punkt 2)

Läser bara (BEGIN READ ONLY, se alfa_db.py). Tick = speldygn. Kolumn -> källa:

PER STAD
 befolkning        settlements.population (nuläge)
 matnetto/tick     (settlement_goods.rate för grain + fish) - befolkning * 0.005
                   = economy.FoodNet (recompute.go; GrainConsumptionPerCitizenPerTick = 0.005, speglad här —
                   ändras konstanten i koden måste FOOD_PER_CITIZEN nedan följa). Rate är RÅ produktion.
 spannmål/fisk     settlement_goods.amount + rate*max(0, world.current_tick - calc_tick)  (= SQL settled())
 magasin (granary) settlement_granary (good_key, amount) — nuläge
 magasin Δ         events 'SitosGranaryStored' (stream_id = stad): payload.granary_after senaste mot det event
                   som ligger närmast --window tick bakåt; coverage_days ur senaste. Ej varje tick.
 silver            settlement_goods silver, samma lazy-formel
 garnison          units där settlement_id = staden: antal män (units.size) med status garrison
 garnisonens förbrukning  senaste 'UpkeepSettled' (stream_id = stad): grain_total, silver_gross per tick.
PER WANAX
 silver totalt, byggnader (buildings), köade byggen (build_queue), enheter (units; antal / män),
 tid till första bygget = första BuildComplete-event (world_tick) minus player_world_records.founded_tick.
EJ MÄTBART I DB
 befolkningskurva och silver över tid per Wanax: bara nuläge lagras (settlements.population,
 settlement_goods). Kurvan byggs av de dagliga körningarna (megaron_alfadata_YYYY-MM-DD.md).
 Världens samlade silver över tid finns dock: events 'SilverAudit' (payload.liquid_total) — visas sist.
"""
import argparse
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import alfa_db  # noqa: E402

FOOD_PER_CITIZEN = 0.005


def f(x, nd=1):
    return "" if x is None else ("%.*f" % (nd, x))


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    alfa_db.add_args(ap)
    ap.add_argument("--window", type=int, default=7, help="magasin Δ: antal tick bakåt (standard 7)")
    args = ap.parse_args(argv)
    db = alfa_db.connect(args)
    world = alfa_db.pick_world(db, args.world)
    W, T = world["id"], world["current_tick"]
    q = lambda sql: db.rows(sql.replace("{W}", W).replace("{T}", str(T)))

    cities = q("""
        SELECT s.id, s.name, s.owner_id, p.username, s.population, s.is_capital, s.control_type,
          (SELECT sum(g.rate) FROM settlement_goods g WHERE g.settlement_id = s.id AND g.good_key IN ('grain','fish')) AS food_rate,
          (SELECT g.amount + g.rate * greatest(0, {T} - g.calc_tick) FROM settlement_goods g WHERE g.settlement_id = s.id AND g.good_key = 'grain') AS grain,
          (SELECT g.amount + g.rate * greatest(0, {T} - g.calc_tick) FROM settlement_goods g WHERE g.settlement_id = s.id AND g.good_key = 'fish') AS fish,
          (SELECT g.amount + g.rate * greatest(0, {T} - g.calc_tick) FROM settlement_goods g WHERE g.settlement_id = s.id AND g.good_key = 'silver') AS silver,
          (SELECT string_agg(good_key || ' ' || round(amount::numeric, 0), ', ' ORDER BY good_key) FROM settlement_granary gr WHERE gr.settlement_id = s.id AND gr.amount > 0) AS granary,
          (SELECT coalesce(sum(u.size), 0) FROM units u WHERE u.settlement_id = s.id AND u.status = 'garrison') AS garrison
        FROM settlements s LEFT JOIN players p ON p.id = s.owner_id
        WHERE s.world_id = '{W}' AND s.state = 'active' AND p.is_ai = false ORDER BY p.username, s.is_capital DESC, s.name""")
    gr_events = q("""SELECT stream_id, world_tick, payload->>'granary_after' AS after, payload->>'coverage_days' AS cov
                     FROM events WHERE world_id = '{W}' AND event_type = 'SitosGranaryStored' ORDER BY world_tick""")
    up_events = q("""SELECT DISTINCT ON (stream_id) stream_id, world_tick, payload->>'grain_total' AS grain, payload->>'silver_gross' AS silver
                     FROM events WHERE world_id = '{W}' AND event_type = 'UpkeepSettled' ORDER BY stream_id, world_tick DESC""")
    gr_by = {}
    for e in gr_events:
        gr_by.setdefault(e["stream_id"], []).append(e)
    up_by = {e["stream_id"]: e for e in up_events}

    rows = []
    for c in cities:
        net = None if c["food_rate"] is None else c["food_rate"] - c["population"] * FOOD_PER_CITIZEN
        evs = gr_by.get(c["id"], [])
        delta = cov = ""
        if evs:
            last = evs[-1]
            old = [e for e in evs if e["world_tick"] <= last["world_tick"] - args.window]
            if old:
                delta = "%+.0f (%d→%d)" % (float(last["after"]) - float(old[-1]["after"]), old[-1]["world_tick"], last["world_tick"])
            cov = f(float(last["cov"]), 0) if last["cov"] is not None else ""
        up = up_by.get(c["id"])
        rows.append([c["username"], c["name"] + (" (huvud)" if c["is_capital"] else ""), c["population"], f(net, 2),
                     f(c["grain"], 0), f(c["fish"], 0), c["granary"] or "", delta or "ej mätbart (<2 event)", cov, f(c["silver"], 0),
                     c["garrison"], "%s säd / %s silver (tick %s)" % (f(float(up["grain"]), 2), f(float(up["silver"]), 2), up["world_tick"]) if up else ""])

    wan = q("""
        SELECT p.id, p.username, r.founded_tick,
          (SELECT sum(g.amount + g.rate * greatest(0, {T} - g.calc_tick)) FROM settlement_goods g JOIN settlements s ON s.id = g.settlement_id
             WHERE s.owner_id = p.id AND s.world_id = '{W}' AND g.good_key = 'silver') AS silver,
          (SELECT count(*) FROM settlements s WHERE s.owner_id = p.id AND s.world_id = '{W}' AND s.state = 'active') AS cities,
          (SELECT count(*) FROM buildings b JOIN settlements s ON s.id = b.settlement_id WHERE s.owner_id = p.id AND s.world_id = '{W}') AS buildings,
          (SELECT count(*) FROM build_queue bq JOIN settlements s ON s.id = bq.settlement_id WHERE s.owner_id = p.id AND bq.world_id = '{W}') AS queued,
          (SELECT count(*) FROM units u WHERE u.owner_id = p.id AND u.world_id = '{W}') AS units,
          (SELECT coalesce(sum(u.size), 0) FROM units u WHERE u.owner_id = p.id AND u.world_id = '{W}') AS men,
          (SELECT min(e.world_tick) FROM events e JOIN settlements s ON s.id = e.stream_id
             WHERE e.world_id = '{W}' AND e.event_type = 'BuildComplete' AND s.owner_id = p.id) AS first_build
        FROM players p JOIN player_world_records r ON r.player_id = p.id AND r.world_id = '{W}'
        WHERE p.is_ai = false ORDER BY r.joined_at""")
    wrows = []
    for w in wan:
        tt = ""
        if w["first_build"] is not None and w["founded_tick"] is not None:
            tt = "%d (grundad %d, bygge klart %d)" % (w["first_build"] - w["founded_tick"], w["founded_tick"], w["first_build"])
        elif w["founded_tick"] is not None:
            tt = "inget bygge klart än (grundad %d)" % w["founded_tick"]
        wrows.append([w["username"], w["cities"], f(w["silver"], 0), w["buildings"], w["queued"],
                      "%s / %s män" % (w["units"], w["men"]), tt])

    audit = q("""SELECT world_tick, payload->>'liquid_total' AS liquid FROM events WHERE world_id = '{W}'
                 AND event_type = 'SilverAudit' ORDER BY world_tick DESC LIMIT 3""")

    print("# Alfa-ekonomi — värld %s (%s), tick %s\n" % (world["name"], W[:8], T))
    print("## Per stad (människors städer)\n")
    print(alfa_db.md_table(["Wanax", "stad", "befolkning", "matnetto/tick", "säd", "fisk", "magasin", "magasin Δ",
                            "täckning (dagar)", "silver", "garnison (män)", "garnisonens förbrukning/tick"], rows) if rows
          else "_inga städer_")
    print("\n## Per Wanax\n")
    print(alfa_db.md_table(["Wanax", "städer", "silver totalt", "byggnader", "köade byggen", "enheter", "tid till första bygget (tick)"], wrows)
          if wrows else "_inga Wanaxer_")
    print("\nVärldens samlade silver (SilverAudit, senaste): " +
          ("; ".join("tick %s: %s" % (a["world_tick"], f(float(a["liquid"]), 0)) for a in audit) if audit else "ej mätbart (inga audit-event)"))
    print("\nEj mätbart i DB: befolkningskurva och silver över tid per Wanax (bara nuläge lagras) — jämför de dagliga körningarna. "
          "Matnetto = rå produktion minus invånare × %s (spegel av economy.FoodNet)." % FOOD_PER_CITIZEN)


if __name__ == "__main__":
    main()
