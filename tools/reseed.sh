#!/usr/bin/env bash
# Reseed av livevärlden — automatiserar megaron_drift.md §Reseed-runbook i ETT kommando.
#
# Varför det här skriptet finns: reseeden har lämnat webben död på ett raderat
# världs-id TVÅ gånger (2026-08-07 och 2026-09-03 — den andra gången ~20 timmars
# drift innan någon märkte det) trots att runbooken redan hade steget nedskrivet.
# En dokumenterad rad räcker inte — se megaron_drift.md
# "En reseed (create-world) kräver ALLTID systemctl restart poleia".
#
# Gör i ordning:
#   0. Pre-flight — stoppar agent-watchdogen lokalt, verifierar att agentflottan är död.
#   1. Bygger cmd/create-world lokalt och skickar binären till servern.
#   2. Kör create-world på servern (TRUNCATE worlds CASCADE + ny värld) och fångar
#      det NYA världs-id:t ur dess utdata.
#   3. Städar zombie-rader i scheduled_events (world_id som inte längre finns i worlds —
#      scheduled_events saknar FK-cascade mot worlds, se runbooken).
#   4. systemctl restart poleia PÅ SERVERN — görs ALDRIG valfritt eller sist i huvudet.
#      ensureWorld() (cmd/server/main.go) cachar världs-id:t vid boot; utan omstart
#      serverar processen ett raderat id tills någon råkar starta om den för hand.
#   5. Verifierar mot journalen: en NY "world ready"-rad med det NYA id:t. Pollar med
#      timeout. Verifierar ALDRIG mot healthz (svarar 200 från den gamla processen)
#      och ALDRIG mot en UUID i HTML:en på / (data-website-id — kristall.infos
#      analysskript, aldrig ändras, ser ut precis som ett världs-id — den fällan gav
#      en falsk mätning 2026-09-04).
#   6. Bekräftar (bästa-försök, varnar men avbryter inte) att scheduled_events har
#      fått KharisTick/UpkeepTick/LoyaltyDecayTick för den nya världen —
#      seedDailyTicks körs asynkront vid boot (main.go: "go seedDailyTicks(...)"),
#      så den kan hinna efter world-ready-loggen.
#
# Runbookens steg 3 (agent-rejoin) och 5 (agentstart) körs INTE av det här skriptet.
# Timothy 2026-07-28: "Sätt inte in agenter!" — de körs för hand, bara på begäran,
# se megaron_drift.md §Reseed-runbook.
#
# --dry-run skriver ut varje steg utan att utföra det eller röra servern.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

REMOTE="${RESEED_REMOTE:-root@10.0.1.92}"
ENV_FILE="${RESEED_ENV_FILE:-/opt/poleia/.env}"
# INGEN default på kartstorleken — med flit. Skriptet raderar livevärlden, och en
# tyst default gör storleken till en gissning i det ögonblick man minst vill gissa.
# Den 2026-09-04 körande världen är 30×30 (900 tiles); runbookens gamla exempel sa
# 230×230 (52 900 tiles) — 58 gånger större. En default hade valt åt operatören.
MAP_WIDTH="${MAP_WIDTH:-}"
MAP_HEIGHT="${MAP_HEIGHT:-}"
AGENT_PATTERN="${RESEED_AGENT_PATTERN:-playtest/agent.py}"
BIN_LOCAL="/tmp/create-world"
BIN_REMOTE="/tmp/create-world"
JOURNAL_TIMEOUT_S="${RESEED_JOURNAL_TIMEOUT_S:-90}"
JOURNAL_POLL_S=3
TICKS_TIMEOUT_S=15
TICKS_POLL_S=3

DRY_RUN=0
ASSUME_YES=0
for arg in "$@"; do
  case "$arg" in
    --dry-run) DRY_RUN=1 ;;
    --yes|-y)  ASSUME_YES=1 ;;
    -h|--help)
      echo "Användning: MAP_WIDTH=<n> MAP_HEIGHT=<n> $0 [--dry-run] [--yes]"
      echo
      echo "  --dry-run  visa varje steg utan att röra något"
      echo "  --yes      hoppa över bekräftelsefrågan (för skriptad körning)"
      echo
      echo "MAP_WIDTH och MAP_HEIGHT är OBLIGATORISKA — skriptet raderar livevärlden"
      echo "och gissar aldrig storleken. Världen 2026-09-04 var 30x30."
      echo "Env: RESEED_REMOTE, RESEED_ENV_FILE, RESEED_AGENT_PATTERN, RESEED_JOURNAL_TIMEOUT_S"
      exit 0
      ;;
    *)
      echo "okänt argument: $arg (se --help)" >&2
      exit 1
      ;;
  esac
done

say() { echo "$*"; }
step() { echo; echo "== $* =="; }

if [ -z "$MAP_WIDTH" ] || [ -z "$MAP_HEIGHT" ]; then
  echo "✗ MAP_WIDTH och MAP_HEIGHT måste anges — skriptet gissar aldrig kartstorleken." >&2
  echo "  Exempel:  MAP_WIDTH=30 MAP_HEIGHT=30 $0 --dry-run" >&2
  exit 1
fi

# Bekräftelse. Steg 2 kör TRUNCATE worlds CASCADE på livevärlden — det finns ingen
# ångra. --dry-run och --yes hoppar över frågan.
if [ "$DRY_RUN" -eq 0 ] && [ "$ASSUME_YES" -eq 0 ]; then
  cat >&2 <<WARN
⚠️  Detta RADERAR den nuvarande världen på $REMOTE och skapar en ny på ${MAP_WIDTH}x${MAP_HEIGHT}.
    Alla städer, enheter, notiser och spelarpositioner försvinner. Det går inte att ångra.
WARN
  printf 'Skriv RESEED för att fortsätta: ' >&2
  read -r confirm
  if [ "$confirm" != "RESEED" ]; then
    echo "avbrutet." >&2
    exit 1
  fi
fi

# remote_sql <SQL> — kör read/write-SQL på servern via psql, med DATABASE_URL
# sourcad ur $ENV_FILE. -tAc = tuple-only, unaligned — bekväm att grep:a/jämföra.
remote_sql() {
  local sql="$1"
  ssh "$REMOTE" "set -a; . '$ENV_FILE'; set +a; psql \"\$DATABASE_URL\" -tAc \"$sql\""
}

if [ "$DRY_RUN" -eq 1 ]; then
  say "*** --dry-run: inget skrivs, ingen server rörs. Visar bara vad som skulle hänt. ***"
fi

step "Steg 0: pre-flight"
if [ "$DRY_RUN" -eq 1 ]; then
  say "  [dry-run] systemctl --user stop agent-watchdog.timer agent-watchdog.service"
  say "  [dry-run] pgrep -f '$AGENT_PATTERN' — och pkill -9 om något träffar"
else
  say "  stoppar agent-watchdog (lokalt)…"
  systemctl --user stop agent-watchdog.timer agent-watchdog.service 2>/dev/null \
    || say "  ⚠ agent-watchdog.timer/.service kunde inte stoppas (kanske redan stoppad/saknas) — fortsätter"

  say "  verifierar att agentflottan är död ('$AGENT_PATTERN')…"
  if pgrep -f "$AGENT_PATTERN" >/dev/null 2>&1; then
    say "  ⚠ levande agentprocesser hittade — pkill -9"
    pkill -9 -f "$AGENT_PATTERN" || true
    sleep 1
    if pgrep -f "$AGENT_PATTERN" >/dev/null 2>&1; then
      echo "  ✗ agentprocesser lever fortfarande efter pkill -9 — avbryter" >&2
      exit 1
    fi
  fi
  say "  ✓ inga agentprocesser lever"
fi

step "Steg 1: bygg create-world och skicka till servern"
if [ "$DRY_RUN" -eq 1 ]; then
  say "  [dry-run] (cd server && go build -o $BIN_LOCAL ./cmd/create-world)"
  say "  [dry-run] scp $BIN_LOCAL $REMOTE:$BIN_REMOTE"
else
  (cd "$ROOT/server" && go build -o "$BIN_LOCAL" ./cmd/create-world)
  scp "$BIN_LOCAL" "$REMOTE:$BIN_REMOTE" >/dev/null
  say "  ✓ byggd och skickad"
fi

step "Steg 2: skapa ny värld (MAP_WIDTH=$MAP_WIDTH MAP_HEIGHT=$MAP_HEIGHT)"
NEW_WORLD_ID="<NEW_WORLD_ID>"
if [ "$DRY_RUN" -eq 1 ]; then
  say "  [dry-run] ssh $REMOTE 'set -a; . $ENV_FILE; set +a; MAP_WIDTH=$MAP_WIDTH MAP_HEIGHT=$MAP_HEIGHT $BIN_REMOTE'"
  say "  [dry-run] (fångar nya världs-id:t ur '✓ World created: <uuid>')"
else
  create_out="$(ssh "$REMOTE" "set -a; . '$ENV_FILE'; set +a; MAP_WIDTH=$MAP_WIDTH MAP_HEIGHT=$MAP_HEIGHT $BIN_REMOTE")"
  say "$create_out"
  NEW_WORLD_ID="$(printf '%s\n' "$create_out" | sed -n 's/.*World created: \(.*\)/\1/p' | tr -d '[:space:]')"
  if [ -z "$NEW_WORLD_ID" ]; then
    echo "  ✗ kunde inte läsa ut världs-id ur create-worlds utdata — avbryter" >&2
    exit 1
  fi
  say "  ✓ ny värld: $NEW_WORLD_ID"
fi

step "Steg 3: städa zombie-rader i scheduled_events"
if [ "$DRY_RUN" -eq 1 ]; then
  say "  [dry-run] DELETE FROM scheduled_events WHERE world_id NOT IN (SELECT id FROM worlds);"
else
  remote_sql "DELETE FROM scheduled_events WHERE world_id NOT IN (SELECT id FROM worlds);" >/dev/null
  say "  ✓ städat"
fi

step "Steg 4: systemctl restart poleia — ALDRIG valfritt"
RESTART_AT=""
if [ "$DRY_RUN" -eq 1 ]; then
  say "  [dry-run] ssh $REMOTE 'systemctl restart poleia'"
else
  # Tidsstämpeln tas PÅ SERVERN, inte lokalt: --since i steg 5 tolkas av serverns
  # journalctl mot serverns klocka. Klockorna gick i takt 2026-09-04, men en drift
  # på några sekunder hade fått steg 5 att missa raden och avbryta en lyckad reseed.
  RESTART_AT="$(ssh "$REMOTE" "date '+%Y-%m-%d %H:%M:%S'")"
  ssh "$REMOTE" "systemctl restart poleia"
  say "  ✓ restart skickad kl $RESTART_AT"
fi

step "Steg 5: verifiera NY 'world ready' i journalen (id=$NEW_WORLD_ID)"
if [ "$DRY_RUN" -eq 1 ]; then
  say "  [dry-run] pollar: ssh $REMOTE \"journalctl -u poleia --no-pager -o cat --since '\$RESTART_AT' | grep 'world ready'\""
  say "  [dry-run] avbryter om ingen rad med det NYA id:t dyker upp inom ${JOURNAL_TIMEOUT_S}s"
  say "  [dry-run] verifierar ALDRIG mot healthz eller data-website-id på /"
else
  elapsed=0
  found=0
  while [ "$elapsed" -lt "$JOURNAL_TIMEOUT_S" ]; do
    line="$(ssh "$REMOTE" "journalctl -u poleia --no-pager -o cat --since '$RESTART_AT'" | grep 'world ready' | tail -1 || true)"
    if [ -n "$line" ] && printf '%s' "$line" | grep -q "$NEW_WORLD_ID"; then
      say "  ✓ $line"
      found=1
      break
    fi
    sleep "$JOURNAL_POLL_S"
    elapsed=$((elapsed + JOURNAL_POLL_S))
  done
  if [ "$found" -ne 1 ]; then
    cat >&2 <<ERR

  ✗✗✗ AVBRUTET — ingen "world ready" med id $NEW_WORLD_ID sågs i journalen inom ${JOURNAL_TIMEOUT_S}s.
  Servern kan fortfarande peka på den gamla, raderade världen — webben är sannolikt DÖD
  (se megaron_drift.md "En reseed kräver ALLTID systemctl restart poleia").
  Verifiera ALDRIG mot healthz eller mot en UUID i HTML:en på / (data-website-id är
  kristall.infos analysskript, inte världs-id:t).
  Kontrollera för hand:
    ssh $REMOTE "systemctl status poleia"
    ssh $REMOTE "journalctl -u poleia --no-pager -o cat --since '$RESTART_AT'" | tail -50
ERR
    exit 1
  fi
fi

step "Steg 6: bekräfta dagstakterna (KharisTick/UpkeepTick/LoyaltyDecayTick) — bästa-försök"
if [ "$DRY_RUN" -eq 1 ]; then
  say "  [dry-run] pollar: SELECT DISTINCT event_type FROM scheduled_events WHERE world_id = '$NEW_WORLD_ID';"
  say "  [dry-run] (seedDailyTicks körs asynkront vid boot — kan hinna efter world-ready-loggen)"
else
  elapsed=0
  have=""
  while [ "$elapsed" -lt "$TICKS_TIMEOUT_S" ]; do
    have="$(remote_sql "SELECT DISTINCT event_type FROM scheduled_events WHERE world_id = '$NEW_WORLD_ID';" || true)"
    if printf '%s' "$have" | grep -q "KharisTick" \
      && printf '%s' "$have" | grep -q "UpkeepTick" \
      && printf '%s' "$have" | grep -q "LoyaltyDecayTick"; then
      say "  ✓ KharisTick/UpkeepTick/LoyaltyDecayTick finns för $NEW_WORLD_ID"
      have="ok"
      break
    fi
    sleep "$TICKS_POLL_S"
    elapsed=$((elapsed + TICKS_POLL_S))
  done
  if [ "$have" != "ok" ]; then
    say "  ⚠ hittade inte alla tre tick-typer inom ${TICKS_TIMEOUT_S}s (seedDailyTicks kan bara vara sen) —"
    say "    kolla för hand: SELECT DISTINCT event_type FROM scheduled_events WHERE world_id = '$NEW_WORLD_ID';"
  fi
fi

step "Klart"
say "Nytt världs-id: $NEW_WORLD_ID"
say "Runbookens steg 3 (agent-rejoin) och 5 (agentstart) INTE körda — gör dem för hand vid behov,"
say "se megaron_drift.md §Reseed-runbook. Kom ihåg: agent-watchdog.timer stoppades i steg 0 —"
say "starta den igen (systemctl --user start agent-watchdog.timer) om flottan ska köra vidare."
