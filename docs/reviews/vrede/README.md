# ORDER25 — R3 temple limits and R5 divine wrath

## Contract and inventory

Base cfd774be, migration202. No active founding grace: founded_tick remains stored,
but current maintenance/projection apply ordinary decay from the start. Prior
punishment rule was <30 with temple immunity. No durable punishment protection
or expiry existed. KharisBandChanged already has owner-scoped durable transition
state and atomic event+notification persistence.

Use kharis_notice_state for a realm shield and a separate decline observation;
no migration. Actual punishment commits loss, shield, event and notification in
one transaction. No loss creates no shield. Seven ticks means tick10 protects
through16 and expires at17; repeats cannot extend it. Existing processed tick
claims still ensure maintenance is not applied twice.

A KharisDeclining warning fires once at a downward crossing of30 only when the
next-tick projected net is negative, and records kharis/net/ticks until15. An
already-low realm does not acquire a fabricated crossing on startup. Positive
or zero net updates observation silently; recovery to>=30 rearms the crossing.
KharisBandChanged stays independent and retains historical meaning. Old outcomes
are not rewritten or retroactively assigned a shield.

R3: ordinary prayers at L1, spontaneous blessings >=60 (L2 or higher). Ceilings
remain50/75/100. Wrath <15, .03 per tick, independent of temples. Owner-private
API protection follows the ORDER24 privacy guard. All four player surfaces and
new divine-wrath Codex article are part of this slice.

Gate: proves understandable cult risk in the main chain. No demolition/rivning,
no new founding grace, no ORDER19, no live writes/merge/push/deploy. Stop for new
canon; hand hash to Claude for integration.

## Evidence

Baseline-red.log: named actual-DB failures for old threshold/temple immunity,
repeat punishment within7ticks, and missing decline warning. New tests cover
exact15 boundary, seven-tick expiry, real maintained temple still punishable,
notification-failure rollback of loss+shield+event, positive-net suppression and
recovery/re-crossing. Additional client/atomicity proofs are recorded below
when complete.

Production checkpoint: `c74c710a`; approved G1 declaration `32e26c16` (base `cfd774be`). No production changes after
physical mutations. No migration or tracked executable change.

- `mutation-shield-red.log`: physically bypassing the shield produces a second
  loss at tick10 (25 grain instead of50).
- `mutation-positive-net-red.log`: physically removing the negative-net guard
  produces an unwanted warning with positive projected net.
- `mutation.json` records byte-exact restored production hashes;
  `mutation-restored-green.log` proves both restored paths with fresh DB tests.
- `targeted-final.log`: actual DB transition/expiry/rollback, owner API date and
  foreign privacy, real CLI status. `packages.log`: whole kharis/handlers/CLI.
- `js-final.log`: all728 JS tests, including new warning, historical punishment
  compatibility, R3/R5 text. `vet.log`: final go vet.
- `full-fresh.log`: final full fresh migration202 Go suite after approved G1
  declaration, checkpoint32e26c16.

## Real client evidence and limits

`run_player_scenario.py` installs a temporary Go test and removes it in finally.
Two clean synthetic two-city realms use actual auth, HTTP handlers, built Keryx,
and Firefox against the real server/assets. A real public KharisTick generates
KharisDeclining (30.1→29.45, net−.65,23 ticks to15). CLI notifications and the
actual dispatch window render that persisted body; no intercepted game API.
Kult displays R3, risk and shield date at native390/1000. Actual API reads leave
stored grain unchanged. Both runs have zero browser page errors.

The shield row for the presentation scenario is explicitly seeded (until107,
current100); this is not claimed as a browser-generated punishment. Actual
punishment/shield creation, seven-tick expiry, temple non-immunity and atomic
rollback are independently proved by the named database tests. These scenarios
start with synthetic cities, not a new join→founding→temple construction journey.
Human comprehension remains TEXT in playtest; no balance claim is made.

`index.html` provides native-size BILD: A1 mobile cult, A2 desktop cult,
A3 actual persisted warning. Inspected at1:1; date and rules visible, no clipped
mobile lines. The existing culture drawer extends below the viewport and scrolls.

Historical failed harness logs remain: `js-targeted.log` used an incorrect export
name (fixed to notifDomain); `player-scenario-stock-attempt.log` assumed grain99
before real tick economy changes. The final harness snapshots actual post-tick
stock before read-only requests. Neither required a production fix.

Founding grace is an inventory finding, not resolved by this slice. No old events
are reinterpreted, no retrospective shield is backfilled. R6/4A is separate: when
removing the cap clamp, review R3 wording against kharis remaining above a lowered
ceiling. ORDER19 and demolition were not started.

## Full-suite architecture finding and approved correction

The first full fresh run (`full-fresh-g1-red.log`) completed with one failing test:
`cmd/server.TestG1ProductionImports`: forbidden `kharis -> tick`. All other
packages passed, including world155.296s. This new import uses the existing
canonical LoadAnchor/WallAt owner for expiry conversion. The explicit G1 list
was updated only after Claude's explicit approval (chat2026-10-23 01:00).
Claude authorized the existing downward edge: tick is below kharis, economy
already consumes it, and canonical time conversion must not be duplicated.
`32e26c16` changes CLAUDE.md and executable g1Allowed together, with no runtime
rule change. Claude explicitly ordered one new full fresh run after this fix.

Final result: full fresh migration202 GREEN26 packages (world155.006s),728 JS GREEN, vet GREEN. Both real-client runs GREEN. No runtime changes after c74c710a; only approved G1 declaration32e26c16.
