# Freight provisions

## Contract

Problem: freighting ships eat from onboard stores but ordinary freight dispatch never loads them; standing routes debit food without stowing it.
Player truth: a freight voyage carries food loaded at its departure port and returns unused goods intact.
Invariant: one ration rule and one provision ledger; no remote port debit and no second standing-route food charge.
Scope: shared economy provision owner, combat adapters, transport departure/release, standing naval routes, tests and owner map.
Non-scope: land expedition prepayment, starvation policy, migration/backfill of live ships, layout.
Acceptance: atomic food shortage rejection; round-trip food aboard; foreign/damaged return keeps existing stores; release returns same goods once; standing naval food charged once from actual departure port.
Stop: new canon or visual design required.
Proof: fresh database baseline/fix, named regression mutations, complete Go suite and relevant vet; existing API/CLI flow coverage. Classification FUNCTION; supports geography → shortage → bronze through physical trade.

## Checkpoint (FOV prioritized by Timothy)
Fresh master baseline transport/combat green. Fresh changed transport/combat/economy/server-architecture green. New lifecycle covers Dispatch/CreateShadow, four outbound kinds, onboard eating, damaged return, same-good release twice and shortage rollback. Not merged/deployed: full suite, mutations, public shortage mapping and standing-route edge review remain.
