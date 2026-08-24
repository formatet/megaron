#!/usr/bin/env python3
"""Mätrigg för megaron_plan_lagersvangningen.md — spannmålslagrets restterm.

Utredningens hypotes (stark, delvis bevisad i planen): grain-tillväxtens
golvdivision (`grainPerCitizen=300` i kharis/tick.go `applyDecay`) låser varje
throttlad stads grain-lager i [0, 300) EFTER varje daglig tick, oavsett hur
stor produktionstakten är. Detta skript mäter det live, i acceptansvärlden,
i stället för att bara läsa koden.

Läser BARA: `worlds`, `settlements`, `settlement_goods`, `settlement_granary`,
`scheduled_events`. Skriver ingenting till databasen. Ändrar ingen produktionskod
och ingen konstant (megaron_arbetssatt.md §15, planens stoppvillkor).

`settled()` är TICK-baserad sedan mig 067 (server/db/migrations/067_tick_
substrate.up.sql): amount + rate*(current_tick - calc_tick). Värdet är alltså
FRUSET mellan två world-ticks — det förändras inte kontinuerligt i realtid.
Att polla oftare än en gång per tick fångar därför inte "mellanlägen", bara
samma frusna värde flera gånger — det är förväntat, inte ett fel i riggen.

Två lägen:

    tools/lagersvangning_matning.py collect [--out DIR] [--poll SECONDS]
        Loopar och skriver en rad per (poll, stad) till goods.csv, plus en
        rad per (poll) till order.csv (KharisTick/UpkeepTick-körordning ur
        scheduled_events, för konsekvenskedjans punkt 2). Ctrl-C avslutar.

    tools/lagersvangning_matning.py analyze --csv DIR
        Läser goods.csv, hittar tick-övergångar (grain_calc_tick ändras),
        räknar ut observerad actual_new (pop-delta) och en analytisk
        rekonstruktion av grain_now/desired_new/predicted_actual_new ur
        planens formel (kharis/tick.go rad ~779-838), och skriver ut en
        sammanfattningstabell: hur ofta lagret verkligen låg i [0,300), hur
        ofta prediktion och observation stämde, och recruit-överkomlighet
        för en spearman-kohort (300 grain, se province/training.go).
"""
import argparse, csv, os, subprocess, sys, time
from datetime import datetime

PROJECT = "megaron-acc"

# spearman: 3 grain/man × 100 man (kohort-rekrytering, MaxUnitSize) = 300.
# province/training.go UnitSpecs["spearman"].Costs["grain"] = 3.
SPEARMAN_COHORT_GRAIN_COST = 300.0

# kharis/tick.go grainPerCitizen (rad ~195) — INTE ändrad, bara läst för att
# räkna ut den analytiska prediktionen. Om konstanten någonsin ändras i koden
# måste den ändras här också för att analysen ska stämma.
GRAIN_PER_CITIZEN = 300.0
STARVATION_POP_LOSS_RATE = 0.005


def psql(sql):
    out = subprocess.run(
        ["docker", "exec", "megaron-acc-postgres-1",
         "psql", "-U", "poleia", "-d", "poleia", "-tAF|", "-c", sql],
        capture_output=True, text=True)
    if out.returncode != 0:
        sys.exit("psql: " + out.stderr.strip())
    return [l.split("|") for l in out.stdout.strip().split("\n") if l.strip()]


def append(path, header, rows):
    new = not os.path.exists(path)
    with open(path, "a", newline="") as f:
        w = csv.writer(f)
        if new:
            w.writerow(header)
        w.writerows(rows)


# En rad per aktiv stad: pop, grain (raw amount+calc_tick+rate OCH settled),
# fish/timber/silver settled (jämförelsevaror utan tillväxtsänka), at_cap-flagga,
# granary-innehåll (grain+fish), och antal distinkta FoodGoods med amount>0
# (= variety-räkningen i growth_calc, för den analytiska rekonstruktionen).
GOODS_SQL = """
WITH w AS (SELECT id, current_tick FROM worlds ORDER BY created_at DESC LIMIT 1)
SELECT w.current_tick, s.name, s.population,
       round(sg.amount::numeric, 4), sg.calc_tick, round(sg.rate::numeric, 4),
       round(settled(sg.amount, sg.rate, sg.calc_tick)::numeric, 4) AS grain_settled,
       CASE WHEN sg.cap > 0 AND settled(sg.amount, sg.rate, sg.calc_tick) >= sg.cap * 0.99
            THEN 1 ELSE 0 END AS grain_at_cap,
       round(settled(sf.amount, sf.rate, sf.calc_tick)::numeric, 1) AS fish_settled,
       round(settled(st.amount, st.rate, st.calc_tick)::numeric, 1) AS timber_settled,
       round(settled(ss.amount, ss.rate, ss.calc_tick)::numeric, 1) AS silver_settled,
       COALESCE((SELECT sum(amount) FROM settlement_granary gr
                 WHERE gr.settlement_id = s.id AND gr.good_key = 'grain'), 0),
       COALESCE((SELECT sum(amount) FROM settlement_granary gr
                 WHERE gr.settlement_id = s.id AND gr.good_key = 'fish'), 0),
       (SELECT count(*) FROM settlement_goods fg
         WHERE fg.settlement_id = s.id
           AND fg.good_key = ANY(ARRAY['grain','fish','livestock','wine','oil'])
           AND COALESCE(fg.amount, 0) > 0) AS food_variety_count
FROM settlements s
CROSS JOIN w
JOIN settlement_goods sg ON sg.settlement_id = s.id AND sg.good_key = 'grain'
LEFT JOIN settlement_goods sf ON sf.settlement_id = s.id AND sf.good_key = 'fish'
LEFT JOIN settlement_goods st ON st.settlement_id = s.id AND st.good_key = 'timber'
LEFT JOIN settlement_goods ss ON ss.settlement_id = s.id AND ss.good_key = 'silver'
WHERE s.world_id = w.id AND s.state = 'active'
ORDER BY s.name;
"""
GOODS_HEADER = [
    "wall", "tick", "settlement", "population",
    "grain_amount_raw", "grain_calc_tick", "grain_rate", "grain_settled", "grain_at_cap",
    "fish_settled", "timber_settled", "silver_settled",
    "granary_grain", "granary_fish", "food_variety_count",
]

# Konsekvenskedjans punkt 2: vilken handler kör före vilken denna due_tick?
# ORDER BY due_tick, id i events.Worker.processBatch (scheduler.go rad 332) ser
# deterministisk ut, men den ordningen sitter i en subquery under en
# UPDATE ... RETURNING (rad 322-336) — Postgres garanterar INTE att RETURNING
# respekterar subqueryns ORDER BY. processed_at (verklig väggklocka) är alltså
# den enda pålitliga källan till faktisk körordning, inte id.
ORDER_SQL = """
SELECT due_tick, event_type, id, processed_at
FROM scheduled_events
WHERE event_type IN ('KharisTick','UpkeepTick')
ORDER BY due_tick DESC, processed_at DESC NULLS LAST
LIMIT 12;
"""
ORDER_HEADER = ["wall", "due_tick", "event_type", "id", "processed_at"]


def collect(out_dir, poll_seconds):
    os.makedirs(out_dir, exist_ok=True)
    goods_path = os.path.join(out_dir, "goods.csv")
    order_path = os.path.join(out_dir, "order.csv")
    print(f"skriver till {out_dir}  (poll var {poll_seconds}s, Ctrl-C avslutar)")
    seen_order_ids = set()
    n = 0
    while True:
        wall = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
        rows = psql(GOODS_SQL)
        rows = [r for r in rows if len(r) > 1]
        append(goods_path, GOODS_HEADER, [[wall] + r for r in rows])

        order_rows = psql(ORDER_SQL)
        new_order_rows = []
        for r in order_rows:
            if len(r) < 4:
                continue
            key = (r[0], r[1], r[2])
            if key in seen_order_ids:
                continue
            seen_order_ids.add(key)
            new_order_rows.append([wall] + r)
        if new_order_rows:
            append(order_path, ORDER_HEADER, new_order_rows)

        n += 1
        tick = rows[0][0] if rows else "?"
        print(f"  [{n}] {wall}  tick {tick}  {len(rows)} städer  "
              f"({len(new_order_rows)} nya order-rader)")
        time.sleep(poll_seconds)


def analyze(csv_dir):
    import collections
    goods_path = os.path.join(csv_dir, "goods.csv")
    with open(goods_path) as f:
        rd = csv.DictReader(f)
        rows = list(rd)

    by_city = collections.defaultdict(list)
    for r in rows:
        by_city[r["settlement"]].append(r)

    print(f"\n=== {goods_path} — {len(rows)} rader, {len(by_city)} städer ===\n")

    for city, crows in by_city.items():
        # En rad per unikt (tick, grain_calc_tick) — dedupa upprepade poll
        # inom samma frusna tick.
        seen = {}
        for r in crows:
            key = int(float(r["grain_calc_tick"]))
            seen.setdefault(key, r)
        ticks = sorted(seen.keys())

        in_band = 0
        total = 0
        recruit_ok = 0
        pred_match = 0
        pred_total = 0
        max_grain = 0.0
        prev = None
        transitions = []
        for t in ticks:
            r = seen[t]
            grain = float(r["grain_settled"])
            pop = int(float(r["population"]))
            max_grain = max(max_grain, grain)
            total += 1
            if 0 <= grain < GRAIN_PER_CITIZEN:
                in_band += 1
            if grain >= SPEARMAN_COHORT_GRAIN_COST:
                recruit_ok += 1

            if prev is not None:
                prev_grain, prev_pop, prev_rate, prev_tick, prev_variety = prev
                dtick = t - prev_tick
                if dtick == 1:
                    # Analytisk rekonstruktion av grain_now (kharis/tick.go
                    # rad 779-838): 1%-decay körs FÖRE growth-CTE:n läser den.
                    grain_now = 0.99 * (prev_grain + prev_rate * dtick)
                    softcap = max(0.0, 1.0 - prev_pop / 30000.0)
                    variety = 1.0 + 0.1 * max(0, prev_variety - 1)
                    desired_new = max(1, round(prev_pop * 0.005 * variety * softcap))
                    if grain_now >= desired_new * GRAIN_PER_CITIZEN:
                        pred_actual_new = desired_new
                        pred_remainder = grain_now - desired_new * GRAIN_PER_CITIZEN
                    else:
                        pred_actual_new = grain_now // GRAIN_PER_CITIZEN
                        pred_remainder = grain_now - pred_actual_new * GRAIN_PER_CITIZEN
                    pred_new_pop = min(30000, max(101, prev_pop + pred_actual_new)) \
                        if grain_now > 0 else \
                        min(30000, max(101, round(prev_pop * (1 - STARVATION_POP_LOSS_RATE))))

                    observed_actual_new = pop - prev_pop
                    pred_total += 1
                    # Matchning inom rundningsfel — upkeep kan ha tagit
                    # ytterligare grain mellan kharis och vår poll (se
                    # order.csv), så remainder kan vara LÄGRE än predicerat,
                    # aldrig högre.
                    if pred_new_pop == pop:
                        pred_match += 1
                    transitions.append((prev_tick, t, prev_pop, pop, observed_actual_new,
                                         round(grain_now, 1), round(pred_remainder, 1), grain))

            prev = (grain, pop, float(r["grain_rate"]), t, int(float(r["food_variety_count"])))

        print(f"-- {city} --")
        print(f"  {total} unika ticks observerade (grain_calc_tick), "
              f"{in_band}/{total} i [0,{int(GRAIN_PER_CITIZEN)}) "
              f"({100*in_band/total:.0f}%), max sett grain={max_grain:.1f}")
        print(f"  recruit (spearman, kräver >={int(SPEARMAN_COHORT_GRAIN_COST)}): "
              f"möjligt vid {recruit_ok}/{total} observerade ticks "
              f"({100*recruit_ok/max(total,1):.0f}%)")
        if pred_total:
            print(f"  analytisk pop-prediktion (nästa tick) matchade observerat "
                  f"{pred_match}/{pred_total} ({100*pred_match/pred_total:.0f}%)")
        if transitions:
            print("  senaste övergångar (prev_tick→tick, pop_prev→pop, Δpop, "
                  "grain_now~, pred_remainder~, obs_grain_settled):")
            for row in transitions[-6:]:
                print(f"    {row[0]}→{row[1]}  pop {row[2]}→{row[3]} (Δ{row[4]})  "
                      f"grain_now~{row[5]}  pred_rest~{row[6]}  obs={row[7]}")
        print()

    order_path = os.path.join(csv_dir, "order.csv")
    if os.path.exists(order_path):
        with open(order_path) as f:
            orows = list(csv.DictReader(f))
        by_tick = collections.defaultdict(dict)
        for r in orows:
            if r["processed_at"]:
                by_tick[r["due_tick"]][r["event_type"]] = r["processed_at"]
        kharis_first = upkeep_first = 0
        for t, d in by_tick.items():
            if "KharisTick" in d and "UpkeepTick" in d:
                if d["KharisTick"] < d["UpkeepTick"]:
                    kharis_first += 1
                else:
                    upkeep_first += 1
        total_pairs = kharis_first + upkeep_first
        print(f"=== körordning KharisTick vs UpkeepTick (order.csv, denna körning) ===")
        if total_pairs:
            print(f"  KharisTick före UpkeepTick: {kharis_first}/{total_pairs} "
                  f"({100*kharis_first/total_pairs:.0f}%)")
            print(f"  UpkeepTick före KharisTick: {upkeep_first}/{total_pairs} "
                  f"({100*upkeep_first/total_pairs:.0f}%)")
        else:
            print("  (för få kompletta par ännu — kör collect längre)")


def main():
    ap = argparse.ArgumentParser(description=__doc__,
                                  formatter_class=argparse.RawDescriptionHelpFormatter)
    sub = ap.add_subparsers(dest="cmd", required=True)

    c = sub.add_parser("collect")
    c.add_argument("--out", default=os.path.expanduser("~/lagersvangning_matning"))
    c.add_argument("--poll", type=int, default=20, help="sekunder mellan polls (default 20)")

    a = sub.add_parser("analyze")
    a.add_argument("--csv", required=True, help="katalogen collect skrev till")

    args = ap.parse_args()
    if args.cmd == "collect":
        collect(args.out, args.poll)
    else:
        analyze(args.csv)


if __name__ == "__main__":
    main()
