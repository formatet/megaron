#!/usr/bin/env bash
# Bevis för migration 104_world_cascade: "ingen rad överlever sin värld".
#
# Körs mot en engångsdatabas i thalassa-postgres-1 — ALDRIG mot poleia eller
# driftservern. Skriver om megaron_test_fk varje körning.
#
# Vad den visar:
#   1. RÖTT vid migration 102 (före fixen):
#      a) en enda "DELETE FROM worlds" på en värld med barnrader i ALLA
#         berörda tabeller + trade_routes går inte ens igenom — den
#         FELAR på en FK-krock (trade_routes NO ACTION blockerar direkt;
#         known_settlements/player_scouted_provinces blockerar via sina
#         egna NO ACTION-kopplingar mot settlements/provinces när de i sin
#         tur cascadar från worlds). Världen går alltså inte att radera
#         rent idag, om man inte manuellt letar upp och tömmer just de
#         tre tabellerna först.
#      b) när de tre blockerarna är manuellt tömda går DELETE igenom, men
#         lämnar tysta liken kvar i build_queue, events, gossip_events,
#         player_scouted_tiles, scheduled_events. (loyalty_events råkar
#         bli 0 ändå via sin egen ON DELETE CASCADE mot settlements — ett
#         indirekt skydd som inte är den varaktiga regeln.)
#   2. Migration 104 körs upp.
#   3. GRÖNT: samma fixtur, samma DELETE FROM worlds — går igenom i ett
#      enda steg, noll kvarvarande rader i alla berörda tabeller.
#   4. Föräldralösa rader sås direkt (world_id pekar på en värld som inte
#      finns) mot 102, sedan migrate upp till 104 — migrationen går igenom
#      (den städar innan den lägger FK:n).
#   5. migrate down 1 → migrate up → inga fel (idempotent i praktiken).
#
# Krav: docker-containern thalassa-postgres-1 kör, ~/go/bin/migrate finns.

set -euo pipefail

CONTAINER=thalassa-postgres-1
DB=megaron_test_fk
DSN="postgres://poleia:poleia@172.18.0.3:5432/${DB}?sslmode=disable"
MIGRATE="$HOME/go/bin/migrate"
MIGRATIONS_PATH="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

psql_c() {
    docker exec -i "$CONTAINER" psql -U poleia -d "$DB" -v ON_ERROR_STOP=1 "$@"
}

echo "=== [setup] Ny engångsdatabas $DB ==="
docker exec "$CONTAINER" psql -U poleia -d postgres -c "DROP DATABASE IF EXISTS ${DB} WITH (FORCE);"
docker exec "$CONTAINER" psql -U poleia -d postgres -c "CREATE DATABASE ${DB};"

echo
echo "=== [1] Migrera till 102 (baslinje, FÖRE fixen) ==="
"$MIGRATE" -path "$MIGRATIONS_PATH" -database "$DSN" goto 102

seed_fixture() {
    # En värld ($1) + en spelare + två städer (i två provinser) + minst en
    # rad i varje berörd tabell, plus trade_routes.
    local world="$1"
    psql_c <<SQL
INSERT INTO worlds (id, name, map_seed, map_width, map_height, status, state)
VALUES ('${world}', 'proof-world-${world}', 1, 10, 10, 'active', 'active');

INSERT INTO players (id, username, email, password_hash)
VALUES ('22222222-2222-2222-2222-222222222222', 'proof-player-${world}', 'proof-${world}@example.com', 'x')
ON CONFLICT (id) DO NOTHING;

INSERT INTO provinces (id, world_id, map_q, map_r, terrain_type)
VALUES ('33333333-3333-3333-3333-333333333333', '${world}', 0, 0, 'plains');

INSERT INTO settlements (id, world_id, province_id, name, culture_id)
VALUES ('44444444-4444-4444-4444-444444444444', '${world}', '33333333-3333-3333-3333-333333333333', 'Proof City', 'achaean');

INSERT INTO provinces (id, world_id, map_q, map_r, terrain_type)
VALUES ('55555555-5555-5555-5555-555555555555', '${world}', 1, 0, 'plains');

INSERT INTO settlements (id, world_id, province_id, name, culture_id)
VALUES ('66666666-6666-6666-6666-666666666666', '${world}', '55555555-5555-5555-5555-555555555555', 'Proof City 2', 'achaean');

-- settlement_id NULL: ingen indirekt väg via settlements ON DELETE CASCADE.
INSERT INTO build_queue (id, world_id, building_type, complete_at, settlement_id)
VALUES (gen_random_uuid(), '${world}', 'granary', now() + interval '1 hour', NULL);

INSERT INTO events (stream_id, stream_type, event_type, payload, world_id)
VALUES ('44444444-4444-4444-4444-444444444444', 'settlement', 'proof_event', '{}'::jsonb, '${world}');

INSERT INTO gossip_events (id, world_id, recipient_id, source_region, category, text, subject_settlement_id)
VALUES (gen_random_uuid(), '${world}', '22222222-2222-2222-2222-222222222222', 'region', 'trade', 'proof gossip', NULL);

INSERT INTO known_settlements (world_id, player_id, settlement_id)
VALUES ('${world}', '22222222-2222-2222-2222-222222222222', '44444444-4444-4444-4444-444444444444');

INSERT INTO loyalty_events (settlement_id, world_id, event_type, loyalty_delta, reason)
VALUES ('44444444-4444-4444-4444-444444444444', '${world}', 'proof', 0, 'proof');

INSERT INTO player_scouted_provinces (world_id, player_id, province_id)
VALUES ('${world}', '22222222-2222-2222-2222-222222222222', '33333333-3333-3333-3333-333333333333');

INSERT INTO player_scouted_tiles (world_id, player_id, q, r)
VALUES ('${world}', '22222222-2222-2222-2222-222222222222', 0, 0);

INSERT INTO scheduled_events (world_id, event_type, payload, process_after)
VALUES ('${world}', 'proof', '{}'::jsonb, now() + interval '1 hour');

INSERT INTO trade_routes (id, world_id, origin_id, destination_id, good_key, quantity, arrives_at)
VALUES (gen_random_uuid(), '${world}', '44444444-4444-4444-4444-444444444444', '66666666-6666-6666-6666-666666666666', 'grain', 10, now() + interval '1 hour');
SQL
}

count_children() {
    local world="$1"
    psql_c -t -A -F' | ' <<SQL
SELECT 'build_queue', count(*) FROM build_queue WHERE world_id = '${world}'
UNION ALL SELECT 'events', count(*) FROM events WHERE world_id = '${world}'
UNION ALL SELECT 'gossip_events', count(*) FROM gossip_events WHERE world_id = '${world}'
UNION ALL SELECT 'known_settlements', count(*) FROM known_settlements WHERE world_id = '${world}'
UNION ALL SELECT 'loyalty_events', count(*) FROM loyalty_events WHERE world_id = '${world}'
UNION ALL SELECT 'player_scouted_provinces', count(*) FROM player_scouted_provinces WHERE world_id = '${world}'
UNION ALL SELECT 'player_scouted_tiles', count(*) FROM player_scouted_tiles WHERE world_id = '${world}'
UNION ALL SELECT 'scheduled_events', count(*) FROM scheduled_events WHERE world_id = '${world}'
UNION ALL SELECT 'trade_routes', count(*) FROM trade_routes WHERE world_id = '${world}'
ORDER BY 1;
SQL
}

WORLD_RED=11111111-1111-1111-1111-111111111111

echo
echo "=== [1a] Så fixtur för värld ${WORLD_RED} (barnrader i alla 8 + trade_routes) ==="
seed_fixture "$WORLD_RED"
echo "Rader före radering:"
count_children "$WORLD_RED"

echo
echo "=== [1b] RÖTT: DELETE FROM worlds i ETT steg — förväntas FELA på FK ==="
set +e
psql_c -c "DELETE FROM worlds WHERE id = '${WORLD_RED}';" 2>&1
DELETE_RC=$?
set -e
if [ "$DELETE_RC" -eq 0 ]; then
    echo "OVÄNTAT: DELETE gick igenom vid migration 102 — plan-antagandet höll inte."
    exit 1
fi
echo "(förväntat fel ovan — DELETE FROM worlds går inte igenom vid 102 pga NO ACTION-FK:er)"

echo
echo "=== [1c] Töm de tre blockerande tabellerna manuellt (den städloop invarianten ska ersätta) ==="
psql_c -c "DELETE FROM trade_routes WHERE world_id = '${WORLD_RED}';"
psql_c -c "DELETE FROM player_scouted_provinces WHERE world_id = '${WORLD_RED}';"
psql_c -c "DELETE FROM known_settlements WHERE world_id = '${WORLD_RED}';"

echo
echo "=== [1d] RÖTT: DELETE FROM worlds går nu igenom, men lämnar lik ==="
psql_c -c "DELETE FROM worlds WHERE id = '${WORLD_RED}';"
echo "Rader kvar efter radering (RÖTT — ska vara >0 i build_queue/events/gossip_events/player_scouted_tiles/scheduled_events):"
count_children "$WORLD_RED"

echo
echo "=== [2] Migrera upp till 104 (fixen) ==="
"$MIGRATE" -path "$MIGRATIONS_PATH" -database "$DSN" up

echo
echo "=== [3] GRÖNT: samma fixtur, samma DELETE, i ETT steg ==="
WORLD_GREEN=77777777-7777-7777-7777-777777777777
seed_fixture "$WORLD_GREEN"
echo "Rader före radering:"
count_children "$WORLD_GREEN"
psql_c -c "DELETE FROM worlds WHERE id = '${WORLD_GREEN}';"
echo "Rader kvar efter radering (GRÖNT — ska vara 0 överallt):"
count_children "$WORLD_GREEN"

echo
echo "=== [4] AC5: föräldralösa rader måste redan finnas när FK:n skapas ==="
echo "--- [4a] Backa till 102, så föräldralösa rader (world_id pekar på en obefintlig värld) ---"
"$MIGRATE" -path "$MIGRATIONS_PATH" -database "$DSN" goto 102
WORLD_GHOST=88888888-8888-8888-8888-888888888888
psql_c <<SQL
INSERT INTO build_queue (id, world_id, building_type, complete_at, settlement_id)
VALUES (gen_random_uuid(), '${WORLD_GHOST}', 'granary', now() + interval '1 hour', NULL);
-- world_tick default (current_world_tick()) looks up the world's own row and
-- returns NULL when it doesn't exist — spell it out explicitly for a ghost world.
INSERT INTO events (stream_id, stream_type, event_type, payload, world_id, world_tick)
VALUES (gen_random_uuid(), 'settlement', 'proof_event', '{}'::jsonb, '${WORLD_GHOST}', 0);
INSERT INTO gossip_events (id, world_id, recipient_id, source_region, category, text)
VALUES (gen_random_uuid(), '${WORLD_GHOST}', '22222222-2222-2222-2222-222222222222', 'region', 'trade', 'ghost gossip');
-- loyalty_events.settlement_id is NOT NULL + FK(settlements) ON DELETE CASCADE,
-- so it can never hold a truly orphaned row under this schema — no ghost row
-- possible here without a real settlement, and that's exactly the point: it's
-- the one of the 8 that's already transitively protected today (see [1d],
-- where its count came back 0 even at migration 102).
INSERT INTO scheduled_events (world_id, event_type, payload, process_after)
VALUES ('${WORLD_GHOST}', 'proof', '{}'::jsonb, now() + interval '1 hour');
SQL
echo "Föräldralösa rader sådda mot en obefintlig värld (${WORLD_GHOST}):"
psql_c -t -A -F' | ' -c "
SELECT 'build_queue', count(*) FROM build_queue WHERE world_id = '${WORLD_GHOST}'
UNION ALL SELECT 'events', count(*) FROM events WHERE world_id = '${WORLD_GHOST}'
UNION ALL SELECT 'gossip_events', count(*) FROM gossip_events WHERE world_id = '${WORLD_GHOST}'
UNION ALL SELECT 'loyalty_events', count(*) FROM loyalty_events WHERE world_id = '${WORLD_GHOST}'
UNION ALL SELECT 'scheduled_events', count(*) FROM scheduled_events WHERE world_id = '${WORLD_GHOST}';
"
echo "--- [4b] migrate up till 104 med föräldralösa rader redan i tabellerna — ska gå igenom (migrationen städar dem själv) ---"
"$MIGRATE" -path "$MIGRATIONS_PATH" -database "$DSN" up
echo "Rader kvar för spökvärlden efter migration 104 (ska vara 0 — migrationens egen DELETE städade dem):"
psql_c -t -A -F' | ' -c "
SELECT 'build_queue', count(*) FROM build_queue WHERE world_id = '${WORLD_GHOST}'
UNION ALL SELECT 'events', count(*) FROM events WHERE world_id = '${WORLD_GHOST}'
UNION ALL SELECT 'gossip_events', count(*) FROM gossip_events WHERE world_id = '${WORLD_GHOST}'
UNION ALL SELECT 'loyalty_events', count(*) FROM loyalty_events WHERE world_id = '${WORLD_GHOST}'
UNION ALL SELECT 'scheduled_events', count(*) FROM scheduled_events WHERE world_id = '${WORLD_GHOST}';
"

echo
echo "=== [5] Idempotens: migrate down 1 → migrate up ==="
"$MIGRATE" -path "$MIGRATIONS_PATH" -database "$DSN" down 1
"$MIGRATE" -path "$MIGRATIONS_PATH" -database "$DSN" up
echo "OK — down/up gick igenom utan fel."

echo
echo "=== [schema-koll] Ingen world_id-tabell utan FK mot worlds efter 104 ==="
psql_c -t -A -c "
SELECT c.table_name FROM information_schema.columns c
WHERE c.column_name = 'world_id' AND c.table_schema = 'public'
EXCEPT
SELECT tc.table_name FROM information_schema.table_constraints tc
JOIN information_schema.constraint_column_usage ccu ON ccu.constraint_name = tc.constraint_name
WHERE tc.constraint_type = 'FOREIGN KEY' AND ccu.table_name = 'worlds'
ORDER BY 1;
"
echo "(tom lista ovan = alla world_id-tabeller har nu en FK mot worlds)"

echo
echo "=== KLART ==="
