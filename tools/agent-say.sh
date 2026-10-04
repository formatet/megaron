#!/usr/bin/env bash
# Send a message between the Codex and Claude sessions working in this repo.
#   tools/agent-say.sh claude "text"   (Codex → Claude: Claude's monitor on .agents/chat.log picks it up)
#   tools/agent-say.sh codex  "text"   (Claude → Codex: also pushed into the live Codex session via `codex queue`)
# Every message is appended to .agents/chat.log, which both agents can read back.
set -euo pipefail
to="${1:?usage: agent-say.sh claude|codex \"message\"}"; shift
msg="$*"; [ -n "$msg" ] || { echo "empty message" >&2; exit 1; }
dir="$(cd "$(dirname "$0")/.." && pwd)/.agents"; mkdir -p "$dir"
case "$to" in
  claude) from=codex ;;
  codex)  from=claude ;;
  *) echo "recipient must be claude or codex" >&2; exit 1 ;;
esac
# one line per message, so a reader can tail the log
printf '[%s] %s→%s: %s\n' "$(date '+%Y-%m-%d %H:%M')" "$from" "$to" "${msg//$'\n'/ ⏎ }" >> "$dir/chat.log"
if [ "$to" = codex ]; then
  thread="$(cat "$dir/codex-thread" 2>/dev/null || true)"
  [ -n "$thread" ] || { echo "no Codex session id in .agents/codex-thread — logged only" >&2; exit 0; }
  codex queue --thread "$thread" --message "[från Claude] $msg (svara: tools/agent-say.sh claude \"…\")" >/dev/null
fi
