# ORDER 23 — prayer standing is a requirement

## Contract

Problem: current prayers both spend standing and gamble their material offering.
Result required: five everyday prayers answer at their standing threshold, consume
traditional or equivalent goods, and leave the stored standing tuple untouched.
Scope: Rite handler, province catalogue, Keryx, Kult and Codex; no migration.
Legacy prayers keep dice/multipliers but stop spending standing. Historical events
retain their original payloads. Realm burden, wrath, demolition and ORDER19 wait.
Gate: proves understandable cult decisions in geography → scarcity → bronze → elite.
Stop for any new canon; hand branch to Claude for review/integration/deploy.

## Baseline and experiments

Base master 1fbffdbe, fresh migration 202. `baseline-red.log` records new actual
HTTP/DB tests failing on old code: five exact-threshold prayers fail with roll99,
short composition accepted, and legacy standing debited. Initial targeted run
also exposed old silent-answer assertions in religion_recipes_test.go; these were
updated to guaranteed outcomes while retaining exact all-goods debit assertions.

Draft 534fa411 was read and applied only for three source files. Its tracked
binary was never applied. Review corrected its unused-but-still-called everyday
dice, misleading verdict, stale comments and overbroad CLI guarantee. Added the
server god_id key to catalogue rows so clients distinguish everyday from legacy.
Composed everyday gifts fail explicitly if valuation is unavailable; positive,
valued goods are required so negative quantities cannot credit stocks.

## Validation

Production checkpoint: `f712df52`, based on `1fbffdbe`. No migration.

- `mutation-restored-green.log`: new relation/threshold/composition/projection tests,
  alpha effects/cooldowns/concurrency/rollback, legacy offering debit and CLI tests
  GREEN on fresh migration202. `handlers-cli.log` retains the earlier failure:
  CLI pointer mismatch and a substitution fixture that forgot Potnia values fish
  at 0.4 (`targeted-before-fixture-fix.log` retains the intermediate attempt). Both were corrected before checkpoint (30 fish equals recipe value12).
- `mutation-red.log`: physically forcing everyday prayers through the legacy dice
  branch makes all five exact-threshold tests RED. `mutate.py` restores bytes in
  finally and reruns the targeted tests on another fresh DB. `mutation.json`
  records the restored source SHA256.
- `player-scenario.log`: TWO clean, separate synthetic-city setups, actual
  authenticated HTTP reads/writes, built Keryx --list and cast with multiplier2,
  actual Firefox Offer → Bring the offering. DB verifies exactly eight grain and
  eight timber consumed across Asterion+Potnia, and unchanged
  stored kharis tuple. No API interception: only initial app bootstrap and map
  markers/actions are fixture scaffolding. Runtime errors: zero. Native 390/1000
  screenshots, inspected at 1:1, are linked in `index.html` (A1–A3).
- `js-final.log`: all 724 JS tests GREEN. `vet.log`: go vet ./... GREEN.
- `full-fresh.log`: final full fresh Go suite GREEN, migration202, 26 packages with tests. See log for exact timings.

Reproduce browser+CLI: `python3 docs/reviews/bon-relation/run_player_scenario.py`.
It installs a temporary Go harness from `player_scenario.go.txt`, starts its own
fresh Postgres using tools/gotest.sh, and always removes the temporary test file.
The browser receives credentials over stdin only; temporary CLI config and binary
are deleted. The normal Go suite has no browser dependency.

## Gates and limits

Code/semantics: actual stored tuple, goods, discrete outcome and cooldowns are
checked; old event payloads and old notification renderers are not rewritten.
Visual: 1:1 Firefox mobile and desktop, both repeated; TEXT understanding awaits
playtest. User: synthetic fixtures exercise real CLI/browser/HTTP/DB, not the
full join → founding → temple construction progression. This report does not
claim that broader progression has been replayed.

All fixtures are isolated; no live writes, merge, push or deploy. Realm burden,
wrath and demolition remain the old rule until their separate orders land.
No claim that those R2/R5/R6 changes are part of this slice. Binary blob remains
byte-identical to master. Extra incense Codex wording was corrected because it
otherwise promised a stronger answer for an oversized everyday gift.

All retained evidence files are SHA256-listed in `SHA256SUMS`; run `python3 docs/reviews/bon-relation/verify.py`.
