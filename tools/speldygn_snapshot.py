#!/usr/bin/env python3
"""Mätriggen för speldygnstestet — ett snapshot var tionde speldygn.

megaron_plan_speldygnstest.md §5: mät FÖRLOPP, inte slutvärde. En körning som
bara rapporterar läget vid dygn 240 har kastat 239 mätpunkter och kan inte säga
VAR något gick sönder.

Skriver tre CSV-filer (append, en rad per snapshot):

  settlements.csv  en rad per bosättning: folk, varor, lager, kharis, armé
  silver.csv       serverns egen silverbokföring (SilverAudit), en rad per tick
  founders.csv     en rad per vandrande host innan den grundat (silverransonen)
  flow.csv         en rad per (händelsetyp eller notiskind) och snapshot —
                   ANTAL SEDAN FÖREGÅENDE SNAPSHOT, inte totalen

Läser bara projektionstabeller och events/notifications. Agentens dagbok är
INTE mätdata (planen §5) — den svarar på fråga 7, friktionen.

    tools/speldygn_snapshot.py                # ett snapshot nu
    tools/speldygn_snapshot.py --out ~/mitt   # annan katalog
    tools/speldygn_snapshot.py --loop 1200    # var 20:e realminut tills ^C
"""
import argparse, csv, os, subprocess, sys, time
from datetime import datetime

PROJECT = "megaron-acc"


def psql(sql):
    out = subprocess.run(
        ["docker", "compose", "-p", PROJECT, "-f", "docker-compose.yml",
         "-f", "docker-compose.acceptance.yml", "exec", "-T", "postgres",
         "psql", "-U", "poleia", "-d", "poleia", "-tAF|", "-c", sql],
        capture_output=True, text=True, cwd=ROOT)
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


# Varje varupost är en lat tupel (amount, rate, calc_tick) — värdet finns inte i
# raden, det RÄKNAS vid läsning. Läser man amount rått mäter man förra
# beräkningen, inte läget (CLAUDE.md, lazy resource eval).
LAZY = "(sg.amount + sg.rate * (w.current_tick - sg.calc_tick))"

SETTLEMENT_SQL = f"""
WITH w AS (SELECT id, current_tick FROM worlds ORDER BY created_at DESC LIMIT 1),
goods AS (
    SELECT sg.settlement_id, sg.good_key,
           GREATEST({LAZY}, 0) AS amt, sg.rate,
           CASE WHEN sg.cap > 0 AND {LAZY} >= sg.cap * 0.99 THEN 1 ELSE 0 END AS at_cap
    FROM settlement_goods sg, w
),
gran AS (SELECT settlement_id, sum(amount) AS stored FROM settlement_granary GROUP BY 1),
army AS (
    SELECT settlement_id, count(*) AS cohorts, sum(size) AS men,
           max(unpaid_periods) AS max_unpaid,
           count(*) FILTER (WHERE reinforcing) AS refilling
    FROM units WHERE category = 'land' AND settlement_id IS NOT NULL GROUP BY 1
)
SELECT w.current_tick, pl.username, s.name, s.population, s.loyalty,
       COALESCE((SELECT round(amt::numeric,1) FROM goods g WHERE g.settlement_id=s.id AND g.good_key='silver'),0),
       COALESCE((SELECT round(amt::numeric,1) FROM goods g WHERE g.settlement_id=s.id AND g.good_key='grain'),0),
       COALESCE((SELECT round(rate::numeric,3) FROM goods g WHERE g.settlement_id=s.id AND g.good_key='grain'),0),
       COALESCE((SELECT round(amt::numeric,1) FROM goods g WHERE g.settlement_id=s.id AND g.good_key='fish'),0),
       COALESCE((SELECT round(amt::numeric,1) FROM goods g WHERE g.settlement_id=s.id AND g.good_key='bronze'),0),
       COALESCE((SELECT sum(at_cap) FROM goods g WHERE g.settlement_id=s.id),0),
       COALESCE((SELECT round(stored::numeric,1) FROM gran WHERE gran.settlement_id=s.id),0),
       COALESCE((SELECT count(*) FROM settlement_placement sp WHERE sp.settlement_id=s.id),0),
       COALESCE((SELECT count(*) FROM buildings b WHERE b.settlement_id=s.id),0),
       COALESCE((SELECT cohorts FROM army a WHERE a.settlement_id=s.id),0),
       COALESCE((SELECT men FROM army a WHERE a.settlement_id=s.id),0),
       COALESCE((SELECT max_unpaid FROM army a WHERE a.settlement_id=s.id),0),
       COALESCE((SELECT refilling FROM army a WHERE a.settlement_id=s.id),0),
       round((pwr.kharis_amount + pwr.kharis_rate * (w.current_tick - pwr.kharis_calc_tick))::numeric, 2),
       round(pwr.kharis_cap::numeric, 1), pwr.cult_level,
       COALESCE((SELECT max(level) FROM temples t WHERE t.settlement_id=s.id),0),
       (SELECT count(*) FROM known_settlements ks WHERE ks.player_id=pl.id),
       (SELECT count(DISTINCT settlement_id) FROM market_snapshots ms WHERE ms.player_id=pl.id)
FROM settlements s
JOIN players pl ON pl.id = s.owner_id
CROSS JOIN w
LEFT JOIN player_world_records pwr ON pwr.player_id = s.owner_id AND pwr.world_id = w.id
WHERE s.world_id = w.id AND s.state = 'active'
ORDER BY pl.username, s.name;
"""

SETTLEMENT_HEADER = [
    "wall", "tick", "player", "settlement", "population", "loyalty",
    "silver", "grain", "grain_rate", "fish", "bronze", "goods_at_cap",
    "granary_stored", "gubbar_placed", "buildings",
    "cohorts", "men", "max_unpaid_periods", "refilling",
    "kharis", "kharis_cap", "cult_level", "temple_level",
    "known_settlements", "contacted_markets",
]

FOUNDER_SQL = """
WITH w AS (SELECT id, current_tick FROM worlds ORDER BY created_at DESC LIMIT 1)
SELECT w.current_tick, pl.username, fp.population,
       round((fp.grain_amount + fp.grain_rate * (w.current_tick - fp.calc_tick))::numeric, 1),
       round((fp.silver_amount + fp.silver_rate * (w.current_tick - fp.calc_tick))::numeric, 1),
       COALESCE((SELECT u.q || ',' || u.r FROM units u
                 WHERE u.id = fp.host_unit_id), '')
FROM founder_phase fp
JOIN players pl ON pl.id = fp.owner_id
CROSS JOIN w
WHERE fp.world_id = w.id AND fp.active
ORDER BY pl.username;
"""

FOUNDER_HEADER = ["wall", "tick", "player", "population", "grain", "silver", "hex"]

# Flödet räknas som DELTA mellan snapshots: "12 desertioner sedan förra
# mätpunkten" säger var något brast, "412 totalt" säger bara att det brast.
FLOW_SQL = """
WITH w AS (SELECT current_tick FROM worlds ORDER BY created_at DESC LIMIT 1)
SELECT 'event', event_type, count(*) FROM events, w
 WHERE world_tick > {since} GROUP BY 2
UNION ALL
SELECT 'notification', kind, count(*) FROM notifications
 WHERE created_at > now() - interval '{secs} seconds' GROUP BY 2
UNION ALL
SELECT 'messenger', 'status_' || status, count(*) FROM messengers GROUP BY 2
UNION ALL
SELECT 'messenger', 'trade_offers_total', count(*) FROM messengers WHERE trade_offer IS NOT NULL
UNION ALL
SELECT 'trade', 'routes_' || CASE WHEN resolved THEN 'resolved' ELSE 'in_transit' END,
       count(*) FROM trade_routes GROUP BY 2;
"""

FLOW_HEADER = ["wall", "tick", "source", "key", "count_since_last"]

# Dödmansgrepp (2026-08-23, se session_2026_08_23_speldygnsrigg.md): en körning
# där ingen spelar ser IDENTISK ut på ytan som en stabil värld — tick, kharis
# och lojalitet fortsätter av sig själva. Bara händelser ett spelardrag
# UTLÖSER kan skilja dem åt. Listan nedan är en TILLÅTELSELISTA över ren
# automatik (körs varje tick utan att någon Wanax rör något) — allt annat
# räknas som spelardrivet, även en händelsetyp som inte fanns när listan
# skrevs. Källa: `select event_type, count(*) from events group by 1` mot en
# avslutad körning, korsad mot var i koden respektive typ emitteras.
AUTOMATIC_EVENT_TYPES = {
    "WorldTick", "SilverAudit", "UpkeepSettled", "KharisMaintained",
    "KharisOffering", "KharisMissedMaintenance", "LoyaltyChanged",
    "DivinePunishment", "UnitAttrition", "StarvationDamage",
}

# Servern bokför redan silvret själv, en rad per tick (SilverAudit). Spak 1
# frågar "var biter deflationen" — den frågan besvaras av den här serien, inte
# av en stadsvis ögonblicksbild: liquid + fund + escrow är hela stocken, och
# mined_since_last är det enda som skapar nytt silver i världen.
SILVER_SQL = """
SELECT world_tick,
       payload->>'liquid_total', payload->>'fund_total', payload->>'escrow_total',
       payload->>'mined_since_last', payload->>'net_delta'
FROM events
WHERE event_type = 'SilverAudit' AND world_tick > {since}
ORDER BY world_tick;
"""

SILVER_HEADER = ["wall", "tick", "liquid", "fund", "escrow", "mined_since_last", "net_delta"]


def snapshot(out_dir, state):
    wall = datetime.now().strftime("%Y-%m-%d %H:%M")
    tick = int(psql("SELECT current_tick FROM worlds ORDER BY created_at DESC LIMIT 1")[0][0])

    rows = psql(SETTLEMENT_SQL)
    append(os.path.join(out_dir, "settlements.csv"), SETTLEMENT_HEADER,
           [[wall] + r for r in rows if len(r) > 1])

    founders = psql(FOUNDER_SQL)
    if founders and len(founders[0]) > 1:
        append(os.path.join(out_dir, "founders.csv"), FOUNDER_HEADER,
               [[wall] + r for r in founders])

    silver = psql(SILVER_SQL.format(since=state.get("tick", -1)))
    if silver and len(silver[0]) > 1:
        append(os.path.join(out_dir, "silver.csv"), SILVER_HEADER,
               [[wall] + r for r in silver])

    since = state.get("tick", -1)
    secs = max(int((time.time() - state.get("wall", time.time())) + 5), 5)
    flow = psql(FLOW_SQL.format(since=since, secs=secs))
    flow_rows = [r for r in flow if len(r) > 1]

    # Dödmansgrepp: summan av spelardrivna händelser sedan förra snapshotet.
    # Samma delta-fönster som resten av flow.csv (since=förra tickens tick) —
    # ingen egen mekanism, bara en filtrering av raderna vi redan hämtat.
    player_events = sum(
        int(r[2]) for r in flow_rows
        if r[0] == "event" and r[1] not in AUTOMATIC_EVENT_TYPES
    )
    flow_rows.append(["player_signal", "player_events_since_last", str(player_events)])

    append(os.path.join(out_dir, "flow.csv"), FLOW_HEADER,
           [[wall, tick] + r for r in flow_rows])

    state["tick"], state["wall"] = tick, time.time()
    print(f"  {wall}  tick {tick}  ·  {len(rows)} bosättningar, "
          f"{len(founders)} vandrande, {len(flow_rows)} flödesrader, "
          f"{player_events} spelarhändelser")

    if player_events == 0:
        state["dead_streak"] = state.get("dead_streak", 0) + 1
        if state["dead_streak"] >= 2:
            print(f"  ⚠⚠ NOLL spelarhändelser i {state['dead_streak']} snapshots i rad "
                  f"(tick {tick}) — något är troligen TRASIGT, inte lugnt. Kolla att "
                  f"agenterna/kadensen faktiskt lever.")
        else:
            print(f"  ⚠ Noll spelarhändelser sedan förra snapshotet (tick {tick}).")
    else:
        state["dead_streak"] = 0

    return tick


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--out", default=os.path.expanduser("~/speldygnstest"))
    ap.add_argument("--loop", type=int, default=0,
                    help="sekunder mellan snapshots (0 = ett enda). 1200 = var 10:e speldygn vid 120 s/tick")
    args = ap.parse_args()

    world = psql("SELECT id FROM worlds ORDER BY created_at DESC LIMIT 1")[0][0]
    out_dir = os.path.join(args.out, world[:8])
    os.makedirs(out_dir, exist_ok=True)
    print(f"värld {world}\nskriver till {out_dir}")

    state = {}
    snapshot(out_dir, state)
    while args.loop:
        time.sleep(args.loop)
        snapshot(out_dir, state)


if __name__ == "__main__":
    ROOT = subprocess.run(["git", "rev-parse", "--show-toplevel"],
                          capture_output=True, text=True).stdout.strip()
    main()
