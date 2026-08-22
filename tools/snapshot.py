#!/usr/bin/env python3
"""Snapshot-riggen — en CSV-rad per bosättning, en gång per anrop.

Varför verktyget finns
──────────────────────
megaron_plan_speldygnstest mäter sex spakar över 240 speldygn. Mätdatan kommer
ur databasen, inte ur agenternas loggar — utan det här skriptet producerar en
lång körning ingen mätdata alls. Bärande princip: mät FÖRLOPP, inte slutvärde.
En rad per bosättning var 10:e speldygn (anroparen styr kadensen, se --watch).

Den enda fällan (megaron_plan_snapshotriggen.md §4)
────────────────────────────────────────────────────
settlement_goods.amount är INTE lagret. Modellen är en lat tupel
(amount, rate, calc_tick); det verkliga lagret är
settled(amount, rate, calc_tick) = amount + rate × (current_tick − calc_tick),
klampat mot [0, cap]. Frågan här använder DB-funktionen settled() — samma
funktion som internal/economy — och klampar exakt som produktionen gör
(LEAST(cap, GREATEST(0, settled(...)))). Läs aldrig amount rakt av.

Bruk
────
    tools/snapshot.py --out run-YYYYMMDD.csv        # ett snapshot, appendar
    tools/snapshot.py --out run.csv --watch 1200     # var 1200:e sekund

Appenda alltid, skriv aldrig över — en dubbelkörning ger dubbla rader (går att
deduplicera på tick i efterhand), aldrig en trasig serie.

Kolumner som INTE kunde fyllas (flaggat, se slutrapporten till planeraren):
  - silver_in_sitos: settlements.sitos_fund_silver revs i migration 106
    ("Sitos-fonden blir ett MAGASIN") — B3 där säger uttryckligen att silver
    ALDRIG går via granaret. Ingen ersättarkolumn finns. Utelämnad helt,
    hellre än ett fält som alltid är tomt.
  - sitos_tithe_cum: ingen egen händelse. B4:s "tithe of the surplus" ÄR
    action.Kind == "store" i EvaluateGranaryAction (internal/economy/sitos.go)
    — samma händelse som sitos_stored_cum. Kolumnen finns kvar (planen bad om
    den) men är alltid identisk med sitos_stored_cum; se slutrapporten.
"""

from __future__ import annotations

import argparse
import csv
import json
import subprocess
import sys
import time
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
ACC = ROOT / "tools" / "acceptance.sh"

# Ordningen matchar megaron_plan_snapshotriggen.md §3, minus silver_in_sitos
# (utelämnad, se modulens docstring och slutrapporten).
FIELDNAMES = [
    "tick",
    "wanax",
    "settlement",
    "population",
    "silver_liquid",
    "insolvent",
    "deserted_cum",
    "grain_amount",
    "grain_rate",
    "pop_growth_10d",
    "goods_at_cap",
    "sitos_stored_cum",
    "sitos_released_cum",
    "sitos_tithe_cum",
    "offers_sent_cum",
    "offers_answered_cum",
    "offers_expired_cum",
    "kharis_pool",
    "rites_cum",
]

# Ett enda SQL-uttryck (§4: N+1-frågor gör skriptet svårt att läsa och lätt att
# ljuga när en bosättning saknar en vara). Resultatet paketeras som JSON i
# själva frågan — psql -tAc ger då en enda rad ren JSON-text, utan de
# avgränsar-krockar en pipe/kommaseparerad rad skulle riskera på namn.
SNAPSHOT_SQL = """
WITH world AS (
    SELECT id, current_tick FROM worlds WHERE status = 'active' LIMIT 1
),
-- ⚠ Den enda fällan (§4): amount är INTE lagret. settled() + klamp mot
-- [0, cap], exakt som internal/economy gör det.
settled_goods AS (
    SELECT sg.settlement_id, sg.good_key,
           LEAST(sg.cap, GREATEST(0, settled(sg.amount, sg.rate, sg.calc_tick))) AS amt,
           sg.rate, sg.cap
    FROM settlement_goods sg
    JOIN settlements s ON s.id = sg.settlement_id
    WHERE s.world_id = (SELECT id FROM world)
),
grain AS (
    SELECT settlement_id, amt AS grain_amount, rate AS grain_rate
    FROM settled_goods WHERE good_key = 'grain'
),
silver AS (
    SELECT settlement_id, amt AS silver_liquid
    FROM settled_goods WHERE good_key = 'silver'
),
cap_count AS (
    SELECT settlement_id, COUNT(*) FILTER (WHERE amt >= 0.99 * cap) AS goods_at_cap
    FROM settled_goods
    GROUP BY settlement_id
),
-- "insolvent" läses ur LIVE-tillståndet (units.unpaid_periods > 0 för
-- enheter denna stad försörjer), inte ur en notis för exakt denna tick.
-- Notiser bär ingen tick-kolumn (bara created_at TIMESTAMPTZ) så "fanns en
-- notis denna tick" går inte att fråga exakt utan en klock/tick-korrelation
-- riggen inte har. Nuvarande obetald-status är samma sanning, mätt direkt.
insolvent AS (
    SELECT support_settlement_id AS settlement_id,
           BOOL_OR(unpaid_periods > 0) AS insolvent
    FROM units
    WHERE world_id = (SELECT id FROM world)
      AND support_settlement_id IS NOT NULL
    GROUP BY support_settlement_id
),
-- UnitDeserted-eventets payload bär bara unit_id (inte settlement_id) —
-- attribueras via units.support_settlement_id, som är PERMANENT för
-- enhetens liv (mig 100-kommentaren: satt vid rekrytering, ändras aldrig).
deserted AS (
    SELECT u.support_settlement_id AS settlement_id, COUNT(*) AS deserted_cum
    FROM events e
    JOIN units u ON u.id = e.stream_id
    WHERE e.world_id = (SELECT id FROM world)
      AND e.event_type = 'UnitDeserted'
      AND e.payload ->> 'reason' = 'silver_shortage'
      AND u.support_settlement_id IS NOT NULL
    GROUP BY u.support_settlement_id
),
sitos AS (
    SELECT stream_id AS settlement_id,
           COUNT(*) FILTER (WHERE event_type = 'SitosGranaryStored') AS sitos_stored_cum,
           COUNT(*) FILTER (WHERE event_type = 'SitosGranaryReleased') AS sitos_released_cum
    FROM events
    WHERE world_id = (SELECT id FROM world)
      AND event_type IN ('SitosGranaryStored', 'SitosGranaryReleased')
    GROUP BY stream_id
),
offers AS (
    SELECT origin_id AS settlement_id,
           COUNT(*) FILTER (WHERE trade_offer IS NOT NULL) AS offers_sent_cum,
           COUNT(*) FILTER (WHERE trade_offer ->> 'status' IN ('accepted', 'declined')) AS offers_answered_cum,
           COUNT(*) FILTER (WHERE trade_offer ->> 'status' = 'expired') AS offers_expired_cum
    FROM messengers
    WHERE world_id = (SELECT id FROM world)
      AND trade_offer IS NOT NULL
    GROUP BY origin_id
),
rites AS (
    SELECT stream_id AS settlement_id, COUNT(*) AS rites_cum
    FROM events
    WHERE world_id = (SELECT id FROM world)
      AND event_type = 'RiteCast'
    GROUP BY stream_id
),
kharis AS (
    SELECT pwr.player_id,
           GREATEST(0, settled(pwr.kharis_amount, pwr.kharis_rate, pwr.kharis_calc_tick)) AS kharis_pool
    FROM player_world_records pwr
    WHERE pwr.world_id = (SELECT id FROM world)
)
SELECT
    (SELECT current_tick FROM world)                    AS tick,
    p.username                                           AS wanax,
    s.name                                                AS settlement,
    s.population                                          AS population,
    COALESCE(silver.silver_liquid, 0)                    AS silver_liquid,
    COALESCE(insolvent.insolvent, false)                 AS insolvent,
    COALESCE(deserted.deserted_cum, 0)                   AS deserted_cum,
    COALESCE(grain.grain_amount, 0)                      AS grain_amount,
    COALESCE(grain.grain_rate, 0)                        AS grain_rate,
    COALESCE(cap_count.goods_at_cap, 0)                  AS goods_at_cap,
    COALESCE(sitos.sitos_stored_cum, 0)                  AS sitos_stored_cum,
    COALESCE(sitos.sitos_released_cum, 0)                AS sitos_released_cum,
    -- Ingen egen händelse — B4:s tithe ÄR "store" (se modulens docstring).
    COALESCE(sitos.sitos_stored_cum, 0)                  AS sitos_tithe_cum,
    COALESCE(offers.offers_sent_cum, 0)                  AS offers_sent_cum,
    COALESCE(offers.offers_answered_cum, 0)              AS offers_answered_cum,
    COALESCE(offers.offers_expired_cum, 0)               AS offers_expired_cum,
    COALESCE(kharis.kharis_pool, 0)                      AS kharis_pool,
    COALESCE(rites.rites_cum, 0)                         AS rites_cum
FROM settlements s
LEFT JOIN players p ON p.id = s.owner_id
LEFT JOIN grain ON grain.settlement_id = s.id
LEFT JOIN silver ON silver.settlement_id = s.id
LEFT JOIN cap_count ON cap_count.settlement_id = s.id
LEFT JOIN insolvent ON insolvent.settlement_id = s.id
LEFT JOIN deserted ON deserted.settlement_id = s.id
LEFT JOIN sitos ON sitos.settlement_id = s.id
LEFT JOIN offers ON offers.settlement_id = s.id
LEFT JOIN rites ON rites.settlement_id = s.id
LEFT JOIN kharis ON kharis.player_id = s.owner_id
WHERE s.world_id = (SELECT id FROM world)
ORDER BY s.name;
"""

# Paketerar hela resultatet som JSON i frågan själv (se kommentaren ovan
# SNAPSHOT_SQL) — psql -tAc ger då EN rad ren JSON-text.
JSON_WRAPPED_SQL = (
    "SELECT COALESCE(json_agg(row_to_json(t)), '[]'::json) FROM ("
    + SNAPSHOT_SQL.strip().rstrip(";")
    + ") t;"
)


def fetch_rows() -> list[dict]:
    """Kör snapshot-frågan via tools/acceptance.sh psql och parsar JSON-svaret.

    Återanvänder acceptance.sh psql i stället för en egen DB-anslutning
    (§2 i planen): det är den enda vägen som garanterat pekar på
    acceptansvärlden, inte på CT126.
    """
    proc = subprocess.run(
        [str(ACC), "psql", JSON_WRAPPED_SQL],
        capture_output=True,
        text=True,
    )
    if proc.returncode != 0:
        raise RuntimeError(
            f"tools/acceptance.sh psql misslyckades (kod {proc.returncode}): "
            f"{proc.stderr.strip()}"
        )
    text = proc.stdout.strip()
    if not text:
        raise RuntimeError("tools/acceptance.sh psql gav tomt svar — kör servern?")
    return json.loads(text)


def load_last_population(out_path: Path) -> dict[tuple[str, str], int]:
    """(wanax, settlement) → population ur SISTA raden för det paret i filen.

    Används för pop_growth_10d: tillväxten sedan FÖREGÅENDE snapshot i den
    här filen (inte nödvändigtvis exakt 10 speldygn — det är anroparens
    kadens, se modulens docstring). Saknas ett tidigare snapshot för paret
    lämnas fältet tomt, inte 0 (0 vore ett falskt påstått nolltillväxt).
    """
    if not out_path.exists() or out_path.stat().st_size == 0:
        return {}
    last: dict[tuple[str, str], int] = {}
    with out_path.open(newline="", encoding="utf-8") as f:
        for row in csv.DictReader(f):
            key = (row["wanax"], row["settlement"])
            try:
                last[key] = int(row["population"])
            except (KeyError, ValueError):
                continue
    return last


def take_snapshot(out_path: Path) -> int:
    """Tar ett snapshot, appendar rader till out_path. Returnerar antal rader."""
    rows = fetch_rows()
    prev_pop = load_last_population(out_path)

    file_is_new = not out_path.exists() or out_path.stat().st_size == 0
    with out_path.open("a", newline="", encoding="utf-8") as f:
        writer = csv.DictWriter(f, fieldnames=FIELDNAMES)
        if file_is_new:
            writer.writeheader()
        for row in rows:
            key = (row["wanax"], row["settlement"])
            population = int(row["population"])
            growth = population - prev_pop[key] if key in prev_pop else ""
            writer.writerow(
                {
                    "tick": row["tick"],
                    "wanax": row["wanax"],
                    "settlement": row["settlement"],
                    "population": population,
                    "silver_liquid": row["silver_liquid"],
                    "insolvent": 1 if row["insolvent"] else 0,
                    "deserted_cum": row["deserted_cum"],
                    "grain_amount": row["grain_amount"],
                    "grain_rate": row["grain_rate"],
                    "pop_growth_10d": growth,
                    "goods_at_cap": row["goods_at_cap"],
                    "sitos_stored_cum": row["sitos_stored_cum"],
                    "sitos_released_cum": row["sitos_released_cum"],
                    "sitos_tithe_cum": row["sitos_tithe_cum"],
                    "offers_sent_cum": row["offers_sent_cum"],
                    "offers_answered_cum": row["offers_answered_cum"],
                    "offers_expired_cum": row["offers_expired_cum"],
                    "kharis_pool": row["kharis_pool"],
                    "rites_cum": row["rites_cum"],
                }
            )
    return len(rows)


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    ap.add_argument("--out", required=True, type=Path, help="CSV-fil att appenda till")
    ap.add_argument(
        "--watch",
        type=int,
        metavar="SEKUNDER",
        help="ta ett snapshot var SEKUNDER:e sekund tills avbrott (Ctrl-C)",
    )
    args = ap.parse_args()

    if not args.watch:
        n = take_snapshot(args.out)
        print(f"snapshot: {n} bosättningar → {args.out}")
        return 0

    print(f"snapshot --watch {args.watch}s → {args.out} (Ctrl-C för att avsluta)")
    try:
        while True:
            try:
                n = take_snapshot(args.out)
                print(f"  {time.strftime('%Y-%m-%d %H:%M:%S')}  {n} bosättningar")
            except Exception as exc:  # noqa: BLE001 — överlev serverns omstart
                # §Framgångskriterier: --watch ska överleva att servern
                # startas om — logga felet, fortsätt, hoppa inte ur loopen.
                print(f"  {time.strftime('%Y-%m-%d %H:%M:%S')}  FEL: {exc}", file=sys.stderr)
            time.sleep(args.watch)
    except KeyboardInterrupt:
        print("\navbruten.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
