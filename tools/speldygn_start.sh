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

# ⛔ Grind mot tyst förgrundning (funnet 2026-08-23: ~/.config/poleia/*.json delas
# med en HELT ANNAN, stående process — isladan/playtest's autonoma agentflotta
# (agent-watchdog.timer, var 10:e minut). Den upptäcker "agenter" genom att bara
# lista filer i samma config-katalog och startar om varje döda process den hittar
# — och tre av fyra spelarnamn här (Ariadne, Daedalos, Sarpedon) råkade redan
# finnas i flottans register. Config-filen den här sviten just skrev pekade
# plötsligt om flottans agent mot ACCEPTANSVÄRLDEN, och flottans founder-fas-kod
# grundar direkt utan att fråga LLM:en — tre spelare stod med färdiga städer
# innan en enda avsedd agent hade spelat sitt första drag. Utan den här kontrollen
# syns det inte förrän någon råkar läsa founders.csv timmar senare.
echo
echo "  kontroll — ingen spelare redan grundad (annan process kan ha kapat configen):"
in_list=""
for n in "${NAMES[@]}"; do
  esc="${n//\'/\'\'}"
  in_list+="${in_list:+,}'$esc'"
done
grounded="$("$ROOT/tools/acceptance.sh" psql \
  "WITH w AS (SELECT id FROM worlds ORDER BY created_at DESC LIMIT 1)
   SELECT pl.username FROM players pl, w
   WHERE pl.username IN ($in_list)
   AND EXISTS (SELECT 1 FROM settlements s WHERE s.owner_id = pl.id AND s.world_id = w.id)
   ORDER BY 1;")"
if [ -n "$grounded" ]; then
  echo "  ✗ redan grundad: $(echo "$grounded" | tr '\n' ' ')" >&2
  cat >&2 <<ERR

  ✗✗✗ AVBRUTET — minst en spelare hade redan en stad innan agenterna fick spela.
  Körningen mäter INTE grundningsvalet om det här händer; se megaron_todo.md /
  session 2026-08-23 för hur det upptäcktes. Vanligaste orsaken: en annan process
  som delar ~/.config/poleia/*.json (t.ex. isladan/playtest's agent-watchdog)
  körde en egen grundning mot den här configen mellan registrering och nu.
  Riv och kör om (tools/acceptance.sh reset), och kontrollera att inget annat
  läser/skriver samma config-filer under tiden.
ERR
  exit 1
fi
echo "  ✓ alla ogrundade — ingen kapning"

cat <<TXT

  igång. Två saker kvar att starta för hand:
    1. mätriggen, i ett eget skal:   tools/speldygn_snapshot.py --loop 1200
    2. agenterna — briefingen bor i megaron_haiku_spelbriefing.md del B

  varje agent kör:  POLEIA_CONFIG=~/.config/poleia/<namn>.json ~/go/bin/keryx <kommando>
TXT
