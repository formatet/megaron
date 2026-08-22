#!/usr/bin/env bash
# Startar speldygnstestet — megaron_plan_speldygnstest.md §4, i ett kommando.
#
#   tools/speldygn_start.sh                          # Talos Daedalos Ariadne Sarpedon
#   tools/speldygn_start.sh Talos Daedalos           # två spelare i stället
#
# Gör i ordning: riv och seeda om världen · registrera spelarna och skriv deras
# keryx-configar · flytta hostarna till SAMMA landmassa · första mätpunkten.
#
# ⛔ Världen MÅSTE seedas om vid start. Tickvakten kör en catch-up-loop
# (internal/tick/worker.go: "catches up one tick at a time"), så en värld som
# stått still spolar ifatt hela nedtiden vid uppstart — en världsdygnsräknare
# som redan står på 100 mäter en ranson som brunnit utan spelare (uppmätt
# 2026-08-23: 97 tick på en tom värld).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
NAMES=("$@")
[ ${#NAMES[@]} -eq 0 ] && NAMES=(Talos Daedalos Ariadne Sarpedon)

command -v "$HOME/go/bin/keryx" >/dev/null || echo "  ⚠ ~/go/bin/keryx saknas — kör: (cd server && go install ./cmd/keryx)"

echo "→ tar takten och kartan ur .env: $(grep -h '^ACC_' "$ROOT/.env" | tr '\n' ' ')"
"$ROOT/tools/acceptance.sh" reset

for n in "${NAMES[@]}"; do
  cfg="$HOME/.config/poleia/$(echo "$n" | tr '[:upper:]' '[:lower:]').json"
  "$ROOT/tools/acceptance.sh" player "$n" "$cfg" >/dev/null
  echo "  ✓ $n → $cfg"
done

echo
"$ROOT/tools/speldygn_placera.py"

echo
echo "  kontroll — alla hostar på samma landmassa:"
"$ROOT/tools/acceptance.sh" psql \
  "SELECT pl.username || '  landmassa ' || mt.landmass_id || '  (' || u.q || ',' || u.r || ')'
   FROM units u JOIN players pl ON pl.id=u.owner_id
   JOIN map_tiles mt ON mt.q=u.q AND mt.r=u.r AND mt.world_id=u.world_id
   WHERE u.type='nomadic_host' ORDER BY 1;" | sed 's/^/    /'

echo
"$ROOT/tools/speldygn_snapshot.py"
cat <<TXT

  igång. Två saker kvar att starta för hand:
    1. mätriggen, i ett eget skal:   tools/speldygn_snapshot.py --loop 1200
    2. agenterna — briefingen bor i megaron_haiku_spelbriefing.md del B

  varje agent kör:  POLEIA_CONFIG=~/.config/poleia/<namn>.json ~/go/bin/keryx <kommando>
TXT
