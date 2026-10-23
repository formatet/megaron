# ORDER21 — filter silver instrumentation at trigger boundary

Contract/base58165dc0. New migration203 up/down only recreates the AFTER UPDATE
trigger with WHEN(NEW.good_key='silver'); down restores179definition. Function,
mining counters, epochs, historical audits, and all tuple writers unchanged.
Proves main-chain instrumentation efficiency without economic change. No new
verb/API/notification/player text: four surfaces unchanged, no BILD needed.

Inventory FIRST: live-inventory.json captured pg_get_triggerdef/function at202,
WHEN null, transaction_read_only=on. Shared tools/alfa_db.py wraps BEGIN READ
ONLY over SSH; no live write. write-paths.txt records runtime matches. Trigger
fires on UPDATE OF amount/rate/cap/calc_tick, including UPSERT conflict UPDATE
and even same-value assignments. Plain INSERT/DELETE do not fire. Runtime
writers never change settlement_goods.good_key. Categories: economy settle/
recompute and food; goods payments/offerings/build/recruit/upkeep; transfer/
trade gifts/loading/unloading/refunds; stock decay/starvation/occupation;
SilverAuditV2 realization. Non-silver updates currently call then early-return.
Guard inside function is retained; only trigger invocation narrowed.

Proof plan: named real DB filtering old-code RED, actual callback probe before
existing guard; expected silver output matrix before/after/down/up, unchanged
function and counters/epoch on migration, rollback; physical migration mutant,
one final full fresh203+JS+vet. No deploy; Claude must restart for SQL-only change.


## Result / proof package

Frozen production checkpoint9b7c982f:203up/down plus tests only. Migration179
and record_silver_mining_credit are byte-for-byte untouched. Up/down only
replace trigger dispatch, preserving function/counter/epoch identities and data.
The inside-function OLD.good_key guard remains as defence. Runtime writers
retain row keys; an artificial key conversion is not an existing game write.

- baseline-red.log: fresh202 callback probe sees silver1/other16; repeat and
  non-silver UPSERT see silver2/other33. Guard prevents credits but not calls.
- targeted-green.log: fresh203 real trigger probe sees silver1/other0 then2/0,
  silver credits6 and unchanged on same-tick repeat. All17 goods in DB covered.
- Callback trace injects one INSERT before the existing function BEGIN guard,
  then executes actual UPDATE/UPSERT. Function replacement and trace table
  live only inside a rolled-back test transaction; never on live.
- Roundtrip SQL down/up/down/up exercises11 numerical cases each: ordinary
  production, clipping/full/over-cap, zero/negative rate, same/backward tick,
  payment not mining, OLD rate and OLD cap. Function text identical, seeded
  counter12.5 and epoch unchanged by every schema switch. Savepoints restore
  each artificial tuple/counter case; final rollback restores whole test DDL.
- Existing179 epoch-reinstall test now reapplies203 inside its transaction,
  preserving its original epoch-reset assertion while leaving current schema
  dispatch for subsequent tests. No production function changed for this.
- mutate.py removes WHEN from actual203up, then migrates a NEW DB: callback
  test RED. Byte-exact restore/upSHA in mutation.json, fresh restored GREEN.
- full-fresh.log/js-final.log/vet.log: final one full fresh203 suite and checks.

## Gates / limits / integration

No player-facing change: same silver and audit outcomes on temenos/keryx/web/
Codex. No new article or BILD; SQL implementation details stay out of player
flows. No timing benchmark or numerical speedup claim; proven reduction is
actual callbacks16→0 for non-silver goods, not inferred from metadata alone.
No game API/browser scenario is claimed: this slice is wholly DB-internal.
Live definition was read only, transaction_read_only=on, migration202. No live
migration, counter edit, restart, merge, push or deploy. Claude must restart
for SQL-only deployment and confirm203/dirty=false/filtered live definition.

Final result: one FULL fresh203Go run GREEN26 packages (world156.639s),734JS GREEN, vet GREEN. Actual callback probe and44silver outcome cases GREEN; physical203WHEN mutant RED→byte-exact restore/fresh GREEN. Runtime remains9b7c982f.
