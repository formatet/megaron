#!/usr/bin/env bash
# Meddelar en ny keryx-release till alla Wanax: noterar commit i client_releases och
# skriver en KeryxReleased-notis per Wanax i varje aktiv värld (en transaktion,
# idempotent per commit). Körs PÅ servern (CT 126) i /opt/poleia, efter att kx
# är uppsläppt som GitHub-releasen keryx-alfa30 — se megaron_drift.md §Release.
#
# Användning: tools/announce-keryx-release.sh <commit>   (7+ tecken, samma som `keryx version`)
# Env: ANNOUNCE_ENV_FILE (default: .env i repo-roten) — läser DATABASE_URL ur den.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENV_FILE="${ANNOUNCE_ENV_FILE:-$ROOT/.env}"
[ $# -eq 1 ] || { echo "Användning: $0 <commit>" >&2; exit 2; }
if [ -z "${DATABASE_URL:-}" ]; then
  [ -r "$ENV_FILE" ] || { echo "Hittar inte $ENV_FILE och DATABASE_URL är inte satt" >&2; exit 1; }
  DATABASE_URL="$(grep -E '^DATABASE_URL=' "$ENV_FILE" | head -1 | cut -d= -f2-)"
  export DATABASE_URL
fi
cd "$ROOT/server"
exec go run ./cmd/announce-keryx-release "$1"
