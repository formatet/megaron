# Keryx for LLM players — proof package (order-keryx-llm-20261110)

Branch `keryx-llm` (from master `612c269d`). Source `server/cmd/keryx/`. No server/API change. Nothing merged, pushed, released or installed.
Files here: `inventory.md` (step 0: every command, aliases, city-naming flags, help word counts), `help-before.txt` (full old root help).

## Help before / after (lines, words of `--help`)
| command | before | after |
|---|---|---|
| root | 104 lines, 1059 words | 38 lines, 150 words |
| dossier | 21, 114 | 21, 114 |
| actions | 30, 136 | 30, 136 |
| city | 25, 129 | 25, 129 |
| build | 25, 191 | 25, 190 |
| place | 28, 159 | 28, 158 |
| staff | 22, 169 | 22, 171 |
| march | 109, 1109 | 94, 940 |
| recruit | 24, 143 | 23, 122 |
| rite | 35, 295 | 35, 292 |
| trade-offer | 28, 222 | 28, 222 |
| transfer | 23, 162 | 22, 156 |

`march` is still long: only the terrain table was removed, as ordered. The remaining prose (explore, land, colonize, conquest mode) is a candidate for a later trim.

## First round for a cold agent (counted by walking the path, not measured with a live agent)
Before: `--help` (91 entries, 104 lines), then to learn the realm `me`, `status`, `city`, `goods`, `idle`/`brief`, `army`, `cities`, `settlements`, `actions`, `actions <category>`, `build --list`, `recruit --list`, then the order itself with `--province <uuid>` found from `settlements` = 15-17 calls (matches the reader agent's figure).
After: `--help` (38 lines) -> `dossier` -> `actions` -> `actions <category>` -> `build --list` (each row ends with the command) -> run that printed line = 6 calls; no id hunting for `trade-accept`/`reply`/`cancel-build`/`call-back`/route verbs (0 extra calls when one candidate exists).

## What changed, per step
1. **Root help = map**: seven cobra groups (Start here, Build and work, Move and fight, Goods and talk, Gods, Look closer, Account), start group with one-line glosses, others as wrapped name lists, the two fixed closing lines. Custom root help function in `cmd_help.go`; every other command keeps cobra's normal help.
   **Hidden (still run):** all 11 `kingdom*` stubs; `me`, `brief`, `idle`, `roster`, `placements`, `forces`, `settlements`, `diplomacy`, `messenger`, `god`, `reports` (admin), `completion`. `wanaxes`/`journal`/`units` were aliases of visible commands and never listed.
   **Kept visible although they overlap `dossier`, and why:** `status` (kharis, surplus-without-sink, temples and ships are not in the dossier), `goods` (full stock and production), `ticklog` (history of a city), `cities` (the only directory of known cities, needed for `--to`/`--dest`), `inbox`/`outbox`/`cargo`/`runners` (they carry the ids and in-flight state), `map`, `hex`, `army`, `sightings`, `places`, `roads`, `storms`, `notifications`, `letters`, `gossip` (each shows something the dossier summarises or omits), `dispatches`/`watch`/`agora`/`password` (account settings).
   `stance`, `load`, `unload`, `repair`, `pickup`, `fit-out`, `lay-up`, `patrol`, `post`, `cancel-training` exist only as `unit` subcommands (no top-level alias exists; adding some would be a new surface) so the Move group prints `unit <list|stance|...>`.
2. **`--city <name-or-id>`** on actions, build, city (+ `placements`), demolish, status, place, staff, disband, idle, map, recruit, rich-offering, ticklog, goods, rite, abandon, transfer, route, message, trade-offer, messenger, market-board, cancel-build. Old `--province`/`--from`/`--settlement` still parse, hidden. The bare name as the only positional works on disband, idle, map, recruit, rich-offering, ticklog, goods, abandon, market-board (and already on build/city/status/place/demolish). **Not** added as positional on transfer/route/message/trade-offer (a bare name could be read as the destination), rite (flags-only by design), actions (`[category]`), staff (`<building>`). One shared helper `addCityFlag` in `cmd_city_target.go`. A wrong name now answers `... — your cities: A, B` (`yourCitiesHint`). `occupation --settlement` is unchanged (it names a city you hold, not one of yours).
3. **Id optional** (`cmd_pick_id.go`): trade-accept, trade-decline, trade-cancel, reply (needs `--text`), cancel-build (searches all your cities unless `--city`), call-back, passage (still needs `--ship`), route-pause (active only), route-resume (paused only), route-delete. One candidate: acts and prints "Using the only ...: label (id ...)" (stderr in `--json`). Several: error listing `keryx <verb> --id <8 chars>  # label` lines; none: "there is no ... to ...". Short unique prefixes are expanded when the value looks hex; any other value goes to the server untouched (so old calls behave exactly as before).
4. **Lists print the command**: `build --list` (` → keryx build --city X --type T [--hex q,r|--hex <q,r>|--god g]`, none at max level), `recruit --list` (only on affordable rows), `rite --list`/cult (` Cast here: keryx rite --city X --prayer P [--storm|--target|--hex|--scope ...] --yes`, only when ready), `actions <category>` (available verbs only; verbs that need a choice start from the list, e.g. `keryx build --city X --list`). Locked rows are unchanged.
5. **Words**: gubbe/gubbar -> citizen(s) in roster, route and status text; `<byggnad>` -> `<building>`; `recruit --men` hidden; float noise fixed (`formatDays` rounds to one decimal, plus harvest, queue and market-board lines); `march`/`redirect`/`unit post`: `--to <city|q,r>` documented (**`--to` now also accepts `q,r`**, a small extension so the one documented form works for hexes), `--q/--r/--target` hidden and working; March terrain table replaced by `Terrain: keryx codex marching`. "tick" check: no printed "tick" found; `day_guard keryx|codex` and `forces_guard` PASS, no exceptions added.
6. **Other surfaces**: Codex `keryx.md` (the round, `--city`, no-id forms, lists that print commands, hidden-but-working views), `marching.md`/`terrain.md` (the passability rules moved to `terrain.md` under "Who can enter what"; `marching.md` is at its word cap, only +12 words were free, so it links to `terrain`), small `--province`/`--from`/`--settlement`/`roster`/`forces` fixes in colonies, citizens, foreign-cities, shipyard, workshop, transfers, units; `articles.json` regenerated, `codex_facts.py --check` clean. Vault: `keryx_playtest.md` section Luna-prompten, `megaron_verblista.md` (three forms).
   **Deployed prompt copy NOT edited:** `/home/tk/.local/share/megaron-luna/luna_prompt.md` is outside repo and vault. The repo copy `tools/playtest/luna_prompt.md` was stale; it is now the deployed text plus four edits (dossier -> actions round; `--city`; no-id forms; `march --help` instead of `unit march --help`; `build --city ... --hex` with `build --list` printing the command). **After merge: `cp tools/playtest/luna_prompt.md ~/.local/share/megaron-luna/luna_prompt.md`.** The fleet's per-agent `kx` binaries must also be replaced after the release.

## Tests added
`cmd_help_test.go` (root golden: groups in order, <= 45 lines, no "kingdom", fixed closing lines; every visible command grouped; every command and subcommand resolves and answers `--help`), `cmd_city_flag_test.go` (table over 23 commands, `--city` before/after positionals, old flags hidden, bare-name positional, wrapper), `cmd_pick_id_test.go` (one/two/none/full id/prefix for trade-accept; reply, route verbs, outbox verbs, cancel-build), `cmd_printed_commands_test.go` (every printed line parses back through its own cobra command; locked rows print none), `cmd_words_test.go` (no Swedish words in any help, march help, `--to q,r`, hidden `--men`, float noise), one wrong-name test in `cmd_resolve_test.go`. Existing tests whose pinned strings changed (`--province`->`--city`, `keryx settlements`->`keryx cities`, cult `Cast here`) were updated.

## Verification (all on this branch)
`cd server && go build ./... && go vet ./cmd/keryx/ && go test -count=1 ./cmd/keryx/` green; `bash tools/test-web.sh` 963 pass / 0 fail (same as master); `tools/gotest.sh ./api/handlers/ -run 'Codex|Catalog'` ok; `python3 tools/codex_facts.py --check` clean; `day_guard.py keryx|codex`, `forces_guard.py` PASS.

## Stopped / not done
Nothing stopped. Not done on purpose: no new top-level `stance`/`load`/... aliases; no `--ship` auto-pick in `passage`; `actions` (no argument) still prints the category overview only, per progressive disclosure; `march` prose beyond the table untouched.
