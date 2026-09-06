#!/usr/bin/env bash
# Acceptansvärlden — isolerad Megaron för hela användarscenariot FÖRE merge.
#
# megaron_arbetssatt §9: den delade levande världen får aldrig vara första
# fullständiga integrationstest. Utan det här skriptet faller den regeln
# tillbaka på dev-servern, vilket är precis vad den förbjuder.
#
#   tools/acceptance.sh up          bygg + starta + vänta tills healthz svarar
#   tools/acceptance.sh reset       riv världen och seeda om (steg 6 i §9)
#   tools/acceptance.sh player NAMN [CONFIG]  registrera + anslut, skriv ut token
#                                   (CONFIG: skriv även en keryx-config dit)
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
# ── Riggen städar efter SIG ────────────────────────────────────────────────────
#
# Varje `up --build` lämnar två otaggade images efter sig: det förra slutimaget
# (~100 MB) och det förra BYGGSTEGET (~260 MB). Riggen byggdes om fem gånger på en
# dag 2026-08-05 och bidrog till att rotpartitionen (100 GB, delad med
# pacman-cachen och hela /var/lib/docker) gick till 99 % — 66 dangling images,
# 12,3 GB. Nio Go-test föll på "No space left on device" och såg ut som
# regressioner tills disken mättes. Ett verktyg som växer monotont varje gång man
# använder det är trasigt, inte bara slarvigt.
#
# ⛔ Varför INTE `docker image prune -f`, som var första förslaget: den är
# SYSTEMOMFATTANDE. Mätt 2026-09-06 låg 53 dangling images på maskinen — 27 i
# riggens storleksklass och 25 i GB-klassen från ANDRA projekt (~13 GB av 15,9 GB
# återvinningsbart). Riggen hade alltså städat fem sjättedelar åt
# kristall/matlista/isladan varje gång någon startade den, och dess beteende hade
# berott på vad som råkade ligga på maskinen. En testrigg städar efter sig själv.
#
# ⚠️ Byggsteget är den STÖRRE läckan (261 MB mot slutimagets 100 MB, mätt
# 2026-09-06). En fix som bara tar slutimaget löser under en tredjedel av
# tillväxten — det såg ut att fungera i första körningen och gjorde det inte.

# docker image inspect .Size rapporterar KOMPRIMERAD storlek under containerd-
# snapshottern — ~3× mindre än vad `docker image ls` visar (mätt: 32 MB mot
# 100 MB för samma image). En städrad som säger 32 när användaren ser 100 i
# `docker image ls` är precis en sådan ljugande statusrad §10 förbjuder, så
# talet läses ur samma källa som ls.
# ⚠️ awk får INTE avsluta tidigt (`exit` efter träff). Gör den det stängs röret
# medan `docker image ls` fortfarande skriver, ls dör av SIGPIPE, och `set -o
# pipefail` gör hela pipelinen 141 — vilket `set -e` tar som fel och river
# riggen mitt i en `up`. Fångat 2026-09-06 genom att köra funktionen skarpt;
# `bash -n` ser det inte. Listan är ~70 rader, så att läsa hela är gratis.
image_disk_size() {
  docker image ls -a --no-trunc --format '{{.ID}} {{.Size}}' 2>/dev/null \
    | awk -v id="$1" '$1==id {v=$2} END {if (v!="") print v}' || true
}

# compose namnger slutimaget deterministiskt <projekt>-<tjänst>. ID:t läses FÖRE
# bygget; efteråt är det gamla otaggat och går inte längre att hitta på namn.
rig_image_id() { docker image inspect -f '{{.Id}}' "${PROJECT}-server" 2>/dev/null || true; }

# Tar bort exakt det slutimage som fanns före ombygget — bara om bygget gav ett
# NYTT id och det gamla nu är otaggat. Docker vägrar ändå ta bort ett image som en
# container använder; kontrollen finns för att utskriften ska vara ärlig i stället
# för att tiga om ett misslyckande.
drop_previous_rig_image() {
  local old="${1:-}" new tags size
  [ -n "$old" ] || return 0
  new=$(rig_image_id)
  [ -n "$new" ] && [ "$new" != "$old" ] || return 0
  tags=$(docker image inspect -f '{{len .RepoTags}}' "$old" 2>/dev/null || echo 1)
  [ "$tags" = "0" ] || return 0
  size=$(image_disk_size "$old")
  if docker image rm "$old" >/dev/null 2>&1; then
    echo "  städat: föregående slutimage (${size:-?})"
  else
    echo "  (föregående slutimage kunde inte tas bort — används det fortfarande?)"
  fi
  return 0
}

# Slänger gamla byggsteg, men BEHÅLLER det nyaste: med classic builder ÄR
# byggstegets lager nästa bygges cache (`docker system df` visar Build Cache 0 B —
# cachen bor i imagelagren, inte i BuildKit). Slänger man det nyaste kompileras
# allt om från noll varje gång, och riggen blir långsammare i stället för mindre.
# Filtret är etiketten ur Dockerfile:n, så bara Megarons egna byggsteg kan träffas.
drop_stale_builder_images() {
  local first=1 id size n=0
  while read -r id; do
    [ -n "$id" ] || continue
    if [ "$first" = 1 ]; then first=0; continue; fi
    size=$(image_disk_size "$id")
    if docker image rm "$id" >/dev/null 2>&1; then
      n=$(( n + 1 ))
      echo "  städat: gammalt byggsteg (${size:-?})"
    fi
  # Sorterar EXPLICIT på tidsstämpel i stället för att lita på `docker image ls`
  # defaultordning: mätt 2026-09-06 kastade den om två images som delade sekund,
  # och då hade den nyaste (nästa bygges cache) slängts i stället för den äldsta.
  done < <(docker image ls --filter "label=com.megaron.build-stage=builder" \
             --filter "dangling=true" --no-trunc --format '{{.CreatedAt}}|{{.ID}}' 2>/dev/null \
             | sort -r | cut -d'|' -f2 || true)
  return 0
}

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
  local prev; prev=$(rig_image_id)
  "${DC[@]}" up -d --build
  drop_previous_rig_image "$prev"
  drop_stale_builder_images
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
  tick      $(docker inspect -f '{{range .Config.Env}}{{println .}}{{end}}' "${PROJECT}-server-1" 2>/dev/null | sed -n 's/^TICK_SECONDS=//p') s     karta $(cmd_psql "SELECT map_width || 'x' || map_height FROM worlds ORDER BY created_at DESC LIMIT 1" | tr -d ' ')

  nästa:    tools/acceptance.sh player Wanax1
EOF
}

# Registrera + anslut till världen. BÅDA stegen behövs: registrering ensam ger
# ett tomt konto utan värld, och en ny spelare ser då noll enheter — det ser ut
# som att spelet är trasigt (funnet i ögonkollen 2026-07-26).
# `keryx login` skickar TOMT lösenord (cmd_login.go) och kan därför inte logga
# in en spelare som registrerats här. Andra argumentet skriver keryx-configen
# direkt i stället — det är den enda vägen in för en spelande agent, och utan
# den fastnar varje körning på samma fälla (funnet 2026-08-23 under
# förberedelsen av speldygnstestet).
cmd_player() {
  local name="${1:?ange ett namn}" cfgpath="${2:-}" w; w=$(world_id)
  local tok
  tok=$(curl -fsS -m 10 -X POST "$API/auth/register" \
        -H 'Content-Type: application/json' \
        -d "{\"username\":\"$name\",\"email\":\"$name@acceptance.local\",\"password\":\"acceptance-pw-123\"}" \
      | python3 -c 'import json,sys; d=json.load(sys.stdin); print(d.get("access_token") or d["token"])')
  curl -fsS -m 20 -X POST "$API/worlds/$w/join" -H "Authorization: Bearer $tok" >/dev/null
  if [ -n "$cfgpath" ]; then
    mkdir -p "$(dirname "$cfgpath")"
    local pid
    pid=$(curl -fsS -m 10 "$API/auth/me" -H "Authorization: Bearer $tok" \
        | python3 -c 'import json,sys; d=json.load(sys.stdin); print(d.get("id") or d.get("player_id") or "")' 2>/dev/null || echo "")
    python3 -c "import json,sys; json.dump({'server':'$BASE','token':'$tok','world_id':'$w','player_id':'$pid','username':'$name'}, open('$cfgpath','w'), indent=2)"
    chmod 600 "$cfgpath"
  fi
  cat <<EOF
  spelare   $name
  värld     $w
  token     $tok
  config    ${cfgpath:-—}

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
