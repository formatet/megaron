#!/usr/bin/env bash
# UserPromptSubmit hook: show Claude any unread messages Codex left in .agents/till-claude.md,
# then move them to .agents/till-claude.read.md so they're shown once.
dir="$(cd "$(dirname "$0")/../.." && pwd)/.agents"
inbox="$dir/till-claude.md"
[ -s "$inbox" ] || exit 0
echo "📨 Meddelande från Codex (.agents/till-claude.md):"
cat "$inbox"
cat "$inbox" >> "$dir/till-claude.read.md"
: > "$inbox"
