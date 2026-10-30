# E5 — remove verified unused code

Order: `.agents/order-effektivisering-20261029.md` §E5. Started by Codex 2026-10-30 10:13 (three
worktrees), Codex hit its quota mid-Go-edit; finished by Claude 2026-10-30. Per-surface contracts:
`go.md`, `js.md`.

## Contract
Problem: unreachable Go functions, unused JavaScript declarations/exports and abandoned CSS increase maintenance work.
Player truth: behavior, routes, visuals and game rules remain unchanged.
Invariant: retain interface/reflection calls, test/tool consumers, dynamic imports and generated CSS classes.
Scope: independently verified Go, JS and CSS removals in separate commits; evidence and reproducible checks.
Non-scope: changing live behavior, gated Kingdoms, historical records, E1–E4.
Stop: ambiguous reachability is retained and documented, not silently deleted.
Gate: infrastructure supporting chain verification; no new player action or canon decision.

## Go
`deadcode ./...` and `deadcode -test ./...` (go1.27.3) on base `26836b0a`: 54 and 18 reports, intersection 16.
Removed 15: `Client.patch`, `countdown`, `die`, `fetchBuiltBuildingLevel` (keryx); `ProvinceHandler.Marches`
(the route is `WorldHandler.Marches`), `loadLaborCapacities` (handlers); three `BeforeTimedArrival` methods
and `stormOccupancy.at` (combat — no interface or caller names the method); `rankedFoodSlotsAt`,
`PopulationCaps` (economy); `Worker.RegisterWithTimeout` + its now-unused `timeouts` map (events — dispatch
uses `DefaultHandlerTimeout`, as every registration already did); `unit.FirstCargoSQL`.
Retained: `kharis.templeTierMultiplier` — documented FAS 4 hook (stop rule: intended deferred implementation).
After: build + vet clean; the intersection is exactly `templeTierMultiplier`; no new dead function appeared
in either mode (no cascade). Codex left the tree non-compiling (`w.timeouts` in `dispatch`, unused
`encoding/json` in `cmd_staff.go`); both fixed here. Pre-existing `gofmt -l` hits on master are untouched.

## JS
Name search over every `web/**/*.{js,mjs,html}` for each `export` declaration.
Deleted (no consumer anywhere): `blitRuns`, `stampUnderConstruction`, `KNOWN_BUILDINGS`, `actorLabel`, and
`ACTOR_LABELS`, which `actorLabel` alone used. Unexported 37 symbols used only in their own file (the
order said 34; three have been added since). Namespace/dynamic importers of those files were checked —
none names the symbols.
Proof: `tools/test-web.sh` 875/875; link probe (`import()` of every `web/static/js/**/*.js` in node) reports
no missing export. Negative control: unexporting `stampBuilding` (used by `render/city.js`) is caught by the
probe (`does not provide an export named 'stampBuilding'`) and turns 7 web tests red.

## CSS
506 classes in `megaron.css`; a class is dead when its name occurs in no tracked `.js/.mjs/.html/.go/.md`
file under `web/` or `server/` and no dynamic prefix (`x-${…}`, `'x-' +`, `x-{{…}}`) covers it.
Dynamic prefixes kept 25 classes (`dc-urg-*`, `food-state-*`, `notif-tier-*`, `world-*`, `trade-*`, …).
Also retained: `htmx-indicator`/`htmx-request` (set by the HTMX library) and `state-*` (`join.html` builds
`state-{{.State}}`). 177 classes dead → 230 distinct rules removed, every one containing a dead class, none
added; section comments emptied by the removal dropped. Families gone: `room-*`, `labor-*`, `recruit-*`,
`tr-*`, `province-*`, `mh-*`, `bq-*`, `dip-bubble*`, `terrain-*` (which also removes seven hard-coded hex
colours outside `:root`).

## Suite
See the merge commit for the fresh-DB full suite result.
