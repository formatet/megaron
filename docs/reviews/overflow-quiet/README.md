# One StorageOverflow notice per ongoing loss (Claude 2026-10-30, before the alfa reset)

Live (dev world): 465 `StorageOverflow` notices, all fish/production; the store hit its cap every 5 ticks (47 of 50 gaps).
Mig 198 treated a loss as ongoing only if the previous tick also lost, so each refill notified again.

Contract: a production overflow notifies only when no production overflow of the same good in the same city happened in the
last `storage_overflow_quiet_ticks()` = 24 ticks (mig 235). Every outcome event is still written; non-production reasons
(delivery, refund, provisions) notify every time as before. Codex `goods.md`.

Proof: `TestStorageOverflowProductionNoticeOncePerOngoingLoss` (5-tick refills → 1 notice/11 events; +23 silent; +24 quiet → new).
Full suite + web 884/0 on a fresh DB. Mutants: window 2 (old rule) → `:207` · window 100000 → `:215` · window 23 → `:211`.
(The first test scaled ticks from the window and let the 100000 mutant survive; ticks are now literal.)

Not done: the storehouse's "one dispatch when full" (NU 1, Codex) replaces this when it lands.
