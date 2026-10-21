# Landing can retain an already queued offer expiry

Contract: live PassageScan458 failed three times and dead-lettered when carrier
landing rebooked an identical scheduled timer. Boarding and actual landing may
predict the same arrival; offer expiry lacks passage generation and collides.
Reuse an identical active OfferExpiry while preserving generation-specific
arrival jobs and all physical witness transactions. No new event interpretation,
TTL, carrier order, sender/recipient rule, UI or schema. Blocks physical contact
and trade. Scope: scheduleCompletion, an explicit shared pending-job helper owned by events,
and regression. The unit-arrival wrapper delegates to the same helper to avoid
copying the queue predicate. Stop if queue semantics must
change globally. Baseline: fresh messenger suite; reproduce same arrival twice.

Baseline fresh193 messenger suite GREEN; new regression RED on baseline with
the exact production unique-constraint error. Fix: events owns explicit
EnsurePendingTickTx under the caller's domain row lock; only identical active
jobs are reused. The unit-arrival wrapper delegates unchanged semantics there.
Ordinary enqueue still rejects duplicates and dead-lettered rows are not hidden.

Targeted events/messenger/combat fresh193 GREEN. Same predicted arrival yields
one expiry and two generation-specific arrivals; changed arrival yields another
expiry. Physical old-enqueue control named RED, restored regression GREEN.
No timer reset or live player order mutation. The old failed PassageScan is
retained for audit; startup had already restored the recurring chain.

Full fresh193 Go suite and final events/messenger/combat vet GREEN. Existing
684JS remain unchanged by this server-only repair.
