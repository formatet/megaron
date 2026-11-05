# Förrådet (Storehouse) — inventory and design (Sonnet 2026-11-04, master 79a6d348, mig 244)

Order `.agents/order-forradet-20261028.md`; allocation rule decided by Timothy 2026-10-29 (`megaron_styrande_beslut.md`): when the store fills,
**all production stops together, the last room is split in proportion to the rates, no good goes first, ONE dispatch when it becomes full.**
Earlier inventory (against mig 228): `/home/tk/wt/codex-storehouse-server/docs/reviews/storehouse/`. Everything in it was re-checked against
this master; the differences are listed under "Changed since the old inventory".

## Contract
Problem: non-food goods each have a technical 1 000 000 ceiling instead of one shared store.
Player truth: one store for everything except food (grain/fish/oil), livestock, horses and cult; 500 with no building, 1 000 / 2 500 / 5 000 at Storehouse level 1-3. Silver and wine count.
Invariants: stock above the ceiling is never cut; new production/deliveries are admitted only into the room that is left; the ceiling follows the building level at every moment (no stored copy to go stale); food/herd/cult unaffected.
Non-scope: food caps, Pen, prices, production rates, horses.

## Solution for the shared ceiling (order point 5)
The ceiling is **computed, never stored** (`storehouse_cap(sid)` = 500 x mult(level) from `buildings`). Level change, build, demolition, conquest, collapse
need no re-capping: the next admission just reads the new level. Overstock is untouchable by construction (`base` = what is held; only the *accrual* is clipped).
- `storehouse_effective(sid, [good, avail])` — pure SQL read at the current tick. Per member: `base` = held stock (a falling stock already netted), `accrual` = pending positive production,
  `room` = max(0, cap - sum(base)); if sum(accrual) <= room all is admitted, else each member gets `room x accrual_i / sum(accrual)`. Order independent.
  Sequential realization (recompute loop, one write at a time) gives exactly the same split as a simultaneous one, because after the first write its accrual is 0 and its admitted share
  has been subtracted from the room (proof by test: `storehouse_test.go`).
- `storage_capped` (already the single gate of production / refining / daily close / silver audit / upkeep) takes the effective cap for pool goods. `settled()` is untouched.
- BEFORE UPDATE trigger `settlement_goods_clip_storehouse` clips accrual smuggled through writers that realize with `settled()` but not through `storage_capped` (payments, loot, tribute, food debits...).
  It only READS sibling rows, so it adds no row lock and no new lock order. Name sorts before `settlement_goods_ledger_upd`, so the ledger books the clip as `overflow`.
- `credit_settlement_good` (every delivery: trade, gift, shipment, transfer, refunds, seized cargo, founder stores...) takes `pg_advisory_xact_lock('storehouse:'||sid)` **before any row lock**,
  realizes its own row, then admits at most the room left in the whole store. Excess is lost with a `StorageOverflow` outcome **exactly like the silo does for food** (verified: mig 191/198 `credit_settlement_good`: no reject, no return, credited amount returned).
  Parallel credits therefore serialize (test).
- Residual race, stated: two *non-credit* realizations of different goods in two concurrent transactions each read the other's unrealized accrual (never its uncommitted write), so together they admit at most the room (they under- rather than over-admit).
  A non-credit realization racing a credit can over-admit by that one accrual. All paths that can race a credit in practice (delivery events) take the lock.

## Where it bites (writers, `goods-writers.txt`)
Production (recompute upsert, stale-rate zero, bronze refining credit, daily close `ledger.CloseDay`, upkeep wages, silver audit v2) all call `storage_capped` -> pooled automatically; no Go call site changed.
Debits (build/recruit/road/tribute/loot/colonist purse/temple offering/food debits) never admit new goods; if they realize accrual with `settled()` the trigger clips it.
Initial stores (`create_metropolis`, `unit_arrival foundColony`, kingdom/settlement helpers) INSERT small founding stocks directly - not clipped (founding stores are below 500).
Silver mining audit (`record_silver_mining_credit`, mig 179/203) used to infer mined silver from OLD.cap; with a pool that inference is false, so it now reads the admitted amount that `storage_capped` / the clip trigger publish in `megaron.silver_admitted`.

## Readers
`settled()` on a pool row can overshoot between realizations (it cannot see siblings). Realization happens at the latest at the daily `CloseDay` for every producing row; the owner-facing reads use `storehouse_effective`
(API goods, `StorageForecast`, `CityForecast`). Payment guards that read `settled()` directly still see unrealized accrual; the trigger makes the payment itself honest (a payment that relied on phantom stock
leaves the row at 0 rather than minting goods).

## Lifecycle
Found/colonise: no row carries a ceiling. Upgrade: next admission reads the new level. Demolish/lost to conquest/collapse: level drops, stock stays, only admission shrinks. Ownership change: buildings move with the city, stock stays.
Backfill: none needed (nothing stored). Existing overstock (rows far above 500) is kept and just stops growing. Down migration restores the mig 238/179 bodies.

## Overflow, dispatch, ledger
Every lost quantity writes the existing `StorageOverflow` event (now with `pool:'storehouse'`, `pool_capacity`, `pool_amount`, `pool_level`; `cap` keeps its per-good meaning).
Production overflow of the store notifies **once per spell** with the same `StorageOverflow` kind carrying `pool:'storehouse'` (no new kind: delivery job, ws, dispatch table and Codex notice article stay as they are; the web/keryx text and the Codex articles name the Storehouse).
Silent while any pooled good lost production inside `storage_overflow_quiet_ticks()` (24) and for further goods of the same realization. Deliveries into a full store notify per outcome as before.
The ledger's `overflow` category comes from the same tx-local counter `storage_capped` and the clip trigger feed, so lost production is `overflow`, never "Other".

## Forecast
`CityForecast` and `StorageForecast` read effective stock for pool goods; the storage forecast projects the pool as ONE aggregate (sum of stock, sum of net, all pooled receipts) against the shared ceiling and gives every pooled good the pool's full time; it never promises each good the same remaining room.

## Changed since the old inventory
Mig 228 -> 240: ledger trigger (238) now books `overflow`; wine left the silo (224); `horses` parked outside the store (order 2026-10-30); the old plan to rewrite a per-row cap was dropped in favour of computing the cap at the gate.
