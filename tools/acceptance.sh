#!/usr/bin/env bash
# Acceptansvärlden — isolerad Megaron för hela användarscenariot FÖRE merge.
#
# megaron_arbetssatt §9: den delade levande världen får aldrig vara första
# fullständiga integrationstest. Utan det här skriptet faller den regeln
# tillbaka på dev-servern, vilket är precis vad den förbjuder.
#
#   tools/acceptance.sh up          bygg + starta + vänta tills healthz svarar
#   tools/acceptance.sh reset       riv världen och seeda om (steg 6 i §9)
#   tools/acceptance.sh player NAMN registrera + anslut, skriv ut token
#   tools/acceptance.sh world       världens id
#   tools/acceptance.sh psql [SQL]  DB-bevis (utan SQL: interaktiv shell)
#   tools/acceptance.sh logs [N]    serverlogg
#   tools/acceptance.sh down        stoppa och radera volymerna
#   tools/acceptance.sh status      vad som kör och mot vilken commit
#
# Allt hamnar på egna portar och egna volymer (docker-compose.acceptance.yml).
# Skriptet vägrar arbeta mot något annat än sitt eget projektnamn.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PROJECT=megaron-acc
BASE=http://localhost:8097
API="$BASE/api/v1"
DC=(docker compose -p "$PROJECT" -f "$ROOT/docker-compose.yml" -f "$ROOT/docker-compose.acceptance.yml")

die() { echo "acceptance: $*" >&2; exit 1; }
have() { command -v "$1" >/dev/null 2>&1; }
have docker || die "docker saknas"
have curl   || die "curl saknas"

# Väntar tills servern svarar FRÅN DEN NYA PROCESSEN. §10 gäller även här:
# healthz innan bygget är klart svarar från den gamla containern, och då mäter
# man föregående körning.
wait_healthy() {
  local tries=${1:-90}
  for ((i = 0; i < tries; i++)); do
    if curl -fsS -m 3 "$BASE/healthz" >/dev/null 2>&1; then return 0; fi
    sleep 2
  done
  die "healthz svarade inte inom $((tries * 2)) s — kör 'tools/acceptance.sh logs'"
}

# Deterministiskt val: nyaste världen efter created_at (den som up/reset just
# seedade), inte "vilken /worlds råkar returnera först". Servern sorterar redan
# created_at DESC, men vi litar inte på den implicita ordningen här — och om mer
# än en värld existerar (t.ex. en gammal körning som inte revs) skriver vi det på
# stderr så kommandon inte tyst pekar på någon annans värld.
world_id() {
  curl -fsS -m 8 "$API/worlds" | python3 -c '
import json, sys
d = json.load(sys.stdin)
ws = d if isinstance(d, list) else d.get("worlds", [])
if not ws: sys.exit("ingen värld seedad")
ws.sort(key=lambda w: (w.get("created_at", ""), w["id"]), reverse=True)
if len(ws) > 1:
    sys.stderr.write("acceptance: %d världar finns — väljer nyaste (%s)\n" % (len(ws), ws[0]["id"]))
print(ws[0]["id"])'
}

cmd_up() {
  echo "→ bygger och startar acceptansvärlden (projekt $PROJECT)"
  "${DC[@]}" up -d --build
  wait_healthy
  local w; w=$(world_id)
  cat <<EOF

  acceptansvärlden kör
  ────────────────────────────────────────────────
  webb      $BASE
  API       $API
  värld     $w
  commit    $(git -C "$ROOT" rev-parse --short HEAD) ($(git -C "$ROOT" rev-parse --abbrev-ref HEAD))
  migration $(cmd_psql "SELECT version || CASE WHEN dirty THEN ' DIRTY' ELSE '' END FROM schema_migrations" | tr -d ' ')
  tick      60 s     karta 30x20

  nästa:    tools/acceptance.sh player Wanax1
EOF
}

# Registrera + anslut till världen. BÅDA stegen behövs: registrering ensam ger
# ett tomt konto utan värld, och en ny spelare ser då noll enheter — det ser ut
# som att spelet är trasigt (funnet i ögonkollen 2026-07-26).
cmd_player() {
  local name="${1:?ange ett namn}" w; w=$(world_id)
  local tok
  tok=$(curl -fsS -m 10 -X POST "$API/auth/register" \
        -H 'Content-Type: application/json' \
        -d "{\"username\":\"$name\",\"email\":\"$name@acceptance.local\",\"password\":\"acceptance-pw-123\"}" \
      | python3 -c 'import json,sys; d=json.load(sys.stdin); print(d.get("access_token") or d["token"])')
  curl -fsS -m 20 -X POST "$API/worlds/$w/join" -H "Authorization: Bearer $tok" >/dev/null
  cat <<EOF
  spelare   $name
  värld     $w
  token     $tok

  export ACC_TOKEN=$tok ACC_WORLD=$w
  curl -s -H "Authorization: Bearer \$ACC_TOKEN" $API/worlds/\$ACC_WORLD/units | python3 -m json.tool
EOF
}

# Steg 6 i §9: återställ mellan de två körningarna. Utan reset kan andra
# körningen lyckas på tillstånd första körningen lämnade efter sig — och då
# bevisar den ingenting.
cmd_reset() {
  echo "→ river världen och seedar om"
  "${DC[@]}" exec -T postgres psql -U poleia -d poleia -c 'TRUNCATE worlds CASCADE; TRUNCATE players CASCADE;' >/dev/null
  "${DC[@]}" restart server >/dev/null
  wait_healthy
  echo "  ny värld: $(world_id)"
}

cmd_psql() {
  if [ $# -eq 0 ]; then "${DC[@]}" exec postgres psql -U poleia -d poleia
  else "${DC[@]}" exec -T postgres psql -U poleia -d poleia -tAc "$*"; fi
}
cmd_logs()   { "${DC[@]}" logs --tail "${1:-80}" server; }
cmd_down()   { "${DC[@]}" down -v; echo "  acceptansvärlden riven, volymerna borta"; }
cmd_status() {
  "${DC[@]}" ps
  curl -fsS -m 3 "$BASE/healthz" >/dev/null 2>&1 \
    && echo "  healthz OK · värld $(world_id) · migration $(cmd_psql 'SELECT version FROM schema_migrations' | tr -d ' ')" \
    || echo "  healthz svarar inte"
}

case "${1:-}" in
  up)     cmd_up ;;
  reset)  cmd_reset ;;
  player) shift; cmd_player "$@" ;;
  world)  world_id ;;
  psql)   shift; cmd_psql "$@" ;;
  logs)   shift; cmd_logs "$@" ;;
  down)   cmd_down ;;
  status) cmd_status ;;
  *)      sed -n '2,20p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//' ;;
esac
