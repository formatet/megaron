# keryx release notice and two small fixes (order-keryx-release-och-sma-20261107)

Branch `keryx-release`. Migration **254** (`client_releases`).

## Commits
1. `keryx hex`: a garrisoned unit now shows its city name (`homes` fetched as in `cmd_unit.go`).
2. Dossier Urgent: `BattleBegun` and `SiegeStarted`. The real siege kind is `SiegeStarted` (`economy/siege_dispatch.go`, `EventSiegeStarted`); there is no `SiegeBegun` and no V2 variant of either kind.
3. temenos: table `client_releases`, `cmd/announce-keryx-release` (Go, SQL in one transaction), `tools/announce-keryx-release.sh`.
4. temenos: `X-Keryx-Latest` response header on `/api/*`, cached 15 s; no row means no header.
5. keryx `version`, stderr line, `KeryxReleased` text in keryx and dossier (Since you last read); web text and icon; Codex `keryx.md` + `keryx` article gets the kind.

## Tests
- `cmd/keryx/cmd_hex_test.go` TestHexNamesTheCityOfAGarrisonedUnit
- `cmd/keryx/cmd_dossier_test.go` TestDossierBattleAndSiegeAreUrgent
- `cmd/announce-keryx-release/main_test.go` idempotent per commit, one notice per Wanax, new commit new notice, archived-world-only Wanax gets none
- `cmd/server/keryxlatest_test.go` header present/absent, cache, DB loader picks newest keryx row
- `cmd/keryx/cmd_release_test.go` comparison table, header remembered by the client, stderr only and once, silent when current or dev, `version` works without login, notice text, dossier placement, Codex URL parity
- `placename.test.mjs` KeryxReleased web text

## Mutants (all red at a `_test.go:NN` line, none `[build failed]`)
| Mutant | Killed at |
|---|---|
| notice written to stdout | cmd_release_test.go:84, :87 |
| comparison always warns | cmd_release_test.go:33, :102 |
| Urgent without BattleBegun | cmd_dossier_test.go:200 |
| Urgent without SiegeStarted | cmd_dossier_test.go:200 |
| `cmd_hex` homes back to nil | cmd_hex_test.go:104 |
| announce without ON CONFLICT | announce-keryx-release/main_test.go:96 |

## Not run
The announce tool was never run against a real database, only the test DB of `tools/gotest.sh`.
