# ORDER26 — temple demolition R6 + ceiling 4A

Contract, base2c4162a7/migration202. Proves understandable cult consequences on
main chain. Voluntary demolition charges10 kharis per finished temple level,
floor1; named god anger notification, exact before/after standing and ceiling
visible before action on web/CLI. Cancellation and hostile sack incur no debt.
Loss/event/notification/delete commit atomically. A lower ceiling never cuts
existing standing. No migration, founding grace, ORDER19, push or deployment.

Inventory: processMaintenance→clampKharis is the sole effective temple-ceiling
clamp. ProjectDailyNet currently forecasts raw gain minus burden; read-side
loadPlayerKharis returns actual standing without cap clamp. cancel_training's
legacy refund LEAST targets raw recordcap, not the temple ceiling; untouched.
Build settles existing standing, prayers read standing, neither clamps it.
Occupation executeSack changes buildings without mutating kharis; must stay so.

Stop on canon conflict. Claude confirmed4A2026-10-23 01:11: above ceiling,
ignore gain and subtract ordinary burden; crossing below the ceiling is allowed. Proofs: baseline named RED,
actual DB DELETE/quote/floor/other cities/cancel/retry/rollback, sack neutrality,
tick/projection above-cap parity, physical mutations, two real client runs,
1:1 screenshots, one final full fresh suite/JS/vet. TEXT remains playtest.

## Result and checkpoints

Runtime `819098db`, clients `7ad311b5`/`c3ed0abc`/`5997b796`. No migration.
Quote and committed outcome use one domain owner, ProjectTempleDemolition.
DELETE rechecks ownership and locks actual building level plus realm standing;
penalty/delete/lift/refund-upgrade/events/notification share one transaction.
Only owners receive demolition_kharis in building lists. Notification persisted
once; push delivers that existing row. A repeated DELETE404 cannot charge twice.

Above ceiling:80/cap50 with gain2.1 and burden.2 becomes79.8, as with gain0.
50.1 becomes49.9; ordinary netted growth resumes on the following tick.
ProjectDailyNet matches; offerings still follow ordinary consumption. Existing
maintained-event raw gain/net fields retain their meaning, while additive
applied_net/standing_before/standing_after/ceiling_rule_version record actual
outcome. No historic event reinterpretation. Blessing threshold unchanged;
R3 now says L2 ceiling lets standing build to60, since retained standing can
remain above a lowered cap.

## Evidence and limits

- `baseline-red.log`: unchanged server leaves80 after L1/L2/L3 demolition and
  emits no anger notice. `ceiling-baseline-red.log`: old tick cliffs80→50 and
  projects+1.9 rather than−.2. Fresh independent databases for both arms.
- `targeted-green.log`: debt/floor/replay, cross-city ceiling quote/private
  owner-only API, notification-failure rollback, real sack no debt/no anger.
- `packages.log`: five whole relevant packages; the only failure was an old
  temple staffing fixture missing player_world_records. Added its realm record;
  `kharis-final.log` proves whole package green. No fallback invented for missing
  authoritative standing. The old per-god demolition fixture likewise now owns
  a real realm record. Original assertions retained.
- `debt-sack.log` preserves a bad new fixture column complete_tick; corrected
  to actual build_queue schema before targeted-green. No production fix.
- Physical mutants: penalty10→0 makes real DELETE debt tests RED; bypassing
  above-ceiling branch makes tick/projection test RED. `mutation.json` records
  byte-exact restore; restored fresh tests GREEN. mutate.py is reproducible.
- `full-fresh.log`, `js-final.log`, `vet.log`: final whole-suite checks at frozen
  runtime819098db. Inspect exit/results; no live world writes/merge/push/deploy.
- `player-scenario.log`: two clean synthetic realms, actual auth/HTTP/compiled
  Keryx/Firefox. CLI no--yes performs no DELETE. Confirmed CLI100→70 and
  cap100→50; inline web confirmation70→60 cap50→40, then actual DELETE.
  Two persisted actual notifications; stock of standing remains60 above cap40.
  Keryx status explains that offerings cannot raise it. Zero JS page errors.
- BILD `index.html`: native390/1000 confirmation, committed outcome and actual
  dispatch. Inspected1:1: no clipped consequences/buttons. Existing city drawer
  scroll remains. No graphics/layout redesign. Human comprehension is TEXT in
  playtest; no new join→founding→construction journey or balance claim.

The browser harness seeds starting cities/temples/standing only, then executes
actual commands without DB edits between them. Main bootstrap/marker list are
scaffolding, all game APIs real. First browser attempt forgot to select the
Buildings tab; its failed log is preserved, corrected harness green twice.

Gate: proves cult decisions understandable on main chain; no founding-grace,
ORDER19 or unrelated economy changes. Claude owns review/merge/deploy.

Final: FULL fresh migration202 GREEN26 packages (world157.369s),733 JS GREEN, final vet GREEN. Both mutations RED→byte-exact restored fresh GREEN. Two actual client scenarios GREEN. Runtime/server/web remain exactly819098db.
