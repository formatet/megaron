#!/usr/bin/env bash
# Kör Go-sviten mot en NYSKAPAD, tom Postgres — det enda utfall som räknas som bevis.
#
# Varför: sviten lämnar rader efter sig, så en återanvänd DB ger gröna tester av fel skäl
# (två transport-tester var gröna i isolering och röda på varje ren rigg, 2026-09-01). Och
# paket som körs parallellt slåss om one_active_world — current_world_tick() ger då NULL
# (sjöslagstestets "flake", mätt 2026-09-23). Därför: färsk DB varje gång och alltid -p 1.
#
# Användning:
#   tools/gotest.sh                          # hela sviten (./...)
#   tools/gotest.sh ./internal/combat/       # ett paket
#   tools/gotest.sh ./api/handlers/ -run TestX -count=5
#   GOTEST_KEEP=1 tools/gotest.sh ...        # låt containern stå kvar efteråt (felsökning)
#
# Miljön rensas (env -i): ett testrecept som ärver en körande servers .env drar med sig
# dess tick-kadens (megaron_arbetssatt §3).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
NAME="megaron-gotest-$$"
PORT="${GOTEST_PORT:-$((56000 + RANDOM % 1000))}"
DSN="postgres://postgres:pw@localhost:$PORT/megaron_test?sslmode=disable"

cleanup() {
  if [ "${GOTEST_KEEP:-0}" = 1 ]; then
    echo "» containern står kvar: $NAME  ($DSN)" >&2
  else
    docker rm -f "$NAME" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

docker run -d --rm --name "$NAME" -e POSTGRES_PASSWORD=pw -p "$PORT:5432" postgres:16-alpine >/dev/null
for _ in $(seq 1 30); do
  docker exec "$NAME" pg_isready -U postgres -q 2>/dev/null && break
  sleep 1
done
docker exec "$NAME" psql -U postgres -qc 'CREATE DATABASE megaron_test;'

# migrate måste köras från server/ — en relativ -path från fel katalog ger en omigrerad DB
# och hundratals falska röda tester.
(cd "$ROOT/server" && migrate -path db/migrations -database "$DSN" up >/dev/null 2>&1) || true
state=$(docker exec "$NAME" psql -U postgres -d megaron_test -tAc 'SELECT version, dirty FROM schema_migrations')
latest=$(ls "$ROOT/server/db/migrations" | sed -n 's/^\([0-9]*\)_.*\.up\.sql$/\1/p' | sort -n | tail -1)
if [ "$state" != "$((10#$latest))|f" ]; then
  echo "✗ schema_migrations = '$state', väntade '$((10#$latest))|f' — avbryter" >&2
  exit 1
fi
echo "» färsk DB på migration $latest · commit $(git -C "$ROOT" rev-parse --short HEAD)$(git -C "$ROOT" diff --quiet || echo '+ändringar')" >&2

args=("$@")
[ ${#args[@]} -eq 0 ] && args=(./...)
cd "$ROOT/server"
env -i HOME="$HOME" PATH="$PATH" GOPATH="${GOPATH:-$HOME/go}" GOCACHE="${GOCACHE:-$HOME/.cache/go-build}" \
  DATABASE_URL="$DSN" go test -count=1 -p 1 "${args[@]}"
