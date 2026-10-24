# BattleBegun — defender start notice

Contract approved by Claude 2026-10-24: level 2 to every defending participant owner plus the active city owner, once per battle and Wanax. New defending Wanaxes joining receive their first notice; existing owners receive no repeat. Names, visible place/hex and clock time only. This slice proves that players can read a battle's beginning on web and Keryx; it does not prove the geography → shortage → bronze → elite playtest chain.

## Behavior and ownership

`initiateOrJoinBattle` remains the shared entry for settlement, field, amphibious, interception and crossing marches. Recipient selection uses defender participant owners plus active settlement ownership at the battle hex; attacking owners are excluded. Several units belonging to one Wanax yield one notice. Opposing Wanax names are deduplicated by player ID, ordered deterministically, and snapshotted in the notice. No login name fallback, troop counts, stance, route, home city, seed or future intention is disclosed. An own participant or own city sees this hex live under the existing visibility rules.

The battle lock serializes joins. The existing personal archive row, identified by kind + world + recipient + payload battle_id, is the durable receipt; reading or muting it does not reset deduplication. No migration or index is required. Notice insertion belongs to the battle transaction; failure rolls back the battle rather than emitting an untrue start. BattleStarted and UnitJoinedBattle audit events now use AppendTx, with chronicle recording after commit. This also prevents an audit start surviving a rolled-back battle. Every production caller buffers only its own invocation's notices and delivers after commit via DeliverCommittedNotification. No second archive insert occurs. As with other committed notifications, a process crash after commit can leave a saved archive notice without a transient live push; reload still finds it.

Web notifications and dispatch share notifText: Wanax first, then place/hex and occurred_at in local clock time. New defending joiners get wording “your forces joined the battle”. The live branch plays the existing own-battle clash, without an extra chime, and refreshes unit/province data. Archive reopening stays silent. Go-to, Codex and per-kind mute reuse the existing generic dispatch controls. Keryx archive and watch share the same readable summary, with explicit ISO wall-clock time. Codex battle explains recipients, joining, sound, archive and mute.

## Validation

- `red-go.log`: the real regression test on old production code fails with zero notices rather than one. Rollback and retry are part of that test.
- `targeted-go.log`: three combat cases cover rollback, retry, one record per owner, city owner without a participating unit, a separate unit owner, reinforcement by an existing owner, a new defending Wanax, reading without resetting deduplication, the actual arrival Handle path, and committed archive+battle visibility at delivery. Keryx tests cover names, place, time, joining and no raw battle ID.
- `full-go.log`: full suite once, 26 tested packages pass (world 153.885s), own new migration-210 database, clean environment and serial packages. Production Go was frozen for the run.
- `full-js.log`: 758 tests pass, including 14 Codex tests and the new BattleBegun rendering test. All four `tools/day_guard.py` surfaces and `go vet ./...` pass.
- `mutation-red.log`: physically remove the archive deduplication condition; the multiple-owner reinforcement test fails with 7 notices instead of 3. Reproduce with `python3 tools/strid_borjar_mutation_20261024.py`. `mutation-restored-green.log`: exact byte restoration and all three BattleBegun combat cases green.

## BILD and provenance

[Review gallery](index.html): two native Firefox captures, 1280 and 390 px, inspected at original size. `bild-final/proof.json` records runtime commit, migration, tick_seconds=1800, source hashes, actual notification and zero page errors. Source hashes match the final production sources; the binary was built from the branch's changed sources on base a5daca39, not claimed to be an unmodified master binary.

The fresh rig's explicit fixture sets two field forces and a due UnitArrival. Normal server worker creates the battle, persists the notice and sends the observed WS frame; no fake notification or substituted dialog. Desktop clicks that actual live chip; mobile opens the saved real archive payload with the application's normal renderer. This is a field battle: its location is a hex rather than an invented city name. Archive API data is saved alongside the actual WS frame. `fixture.sql` and the reusable `tools/strid_borjar_bild_20261024.py` reproduce setup. All owned PG/Redis/server processes are removed by finally; shared acceptance and production/alpha are untouched.

First attempt `bild/` and `bild-run.log` are retained openly: worker/WS succeeded, then the harness timed out using retired `.notif-chip`; the actual class is `.dispatch-chip`. The corrected `bild-final/` run passes. Only final images are selected for review. BILD is delivered for Timothy's judgment; no visual approval is claimed.

No merge, push or deploy. Claude integrates and reviews the branch.
