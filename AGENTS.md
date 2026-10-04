# AGENTS.md — Megaron

**Read `CLAUDE.md` in full before any work.** It is the project instruction file for *every* agent,
not just Claude — the vault pointers, the gate, the four surfaces, the architecture rules and the
deploy steps all apply to you. Where it says "Claude", read "the agent". Its memory paths
(`~/.claude/projects/…/memory/`) are Claude's own; you don't need them. The vault
(`~/Dokument/myltavault`, index `megaron_moc.md`) is shared and is the source of truth.

## Two agents share this repo (Codex + Claude Code)

Timothy runs Codex and Claude Code on the same machine, often at the same time. Rules:

1. **One working tree per agent.** Never both in `~/Projects/megaron` at once. Whoever starts second
   works in a git worktree (`git worktree add ../megaron-<slice> -b <branch>`, or `codex --worktree`).
   Use `git -C <path>`, never a bare `cd` + git. Run `git status` + `git worktree list` before you start
   — uncommitted changes you didn't make belong to the other agent: **don't touch, stash or commit them.**
2. **Claim before you work.** Add a line to `.agents/board.md` (gitignored, local only):
   `(YYYY-MM-DD HH:MM) codex|claude · <slice> · <worktree/branch> · <files/packages touched>`.
   Read it first; if your slice overlaps a claimed package, message the other agent before editing.
   Remove your line when the slice is merged or abandoned.
3. **Messages.**
   - To Claude: append to `.agents/till-claude.md` (Claude sees it automatically on its next turn).
   - To Codex: `codex queue --thread <session-id> --message "…"` — the session id is on the `codex`
     line in `.agents/board.md`. (Codex: write your id there when you claim; `codex agents` lists it.)
   - Keep messages short: what, which branch/commit, what you need back.
4. **Merging to master** is one agent at a time: `git fetch` + rebase on master first, run the relevant
   tests, merge, push, then note the commit hash on the board. Push/deploy steps are in `CLAUDE.md`.
5. **Second opinion.** Either agent may ask the other for a read-only review:
   `codex exec review` (Claude → Codex) or `claude -p "<review prompt>"` (Codex → Claude).
   A review is advice; the agent that owns the slice decides.
6. **`megaron_todo.md` in the vault** is shared: edit only your own lines, re-read immediately before
   writing (the other agent may have changed it).
