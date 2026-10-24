# Correspondence K1/return-post/K2–K4 integration

Timothy2026-10-24: “merga och fixa själv, claude vilar”. Integration in an owned branch; five dependency-ordered merges are conflict-free and their initial resulting tree equals K4. Main/master and live untouched during validation.

## Contract and review corrections

Proves the trade-chain gate through an actionable queue, same-bearer return post, historical personal archive and private memory. No presence/receipts/counteroffer/new FOW or K7 contract is added.

Independent server review plus integrator review found two real blockers: status-only masking still exposed ETA/ship/future resolution fields pre-return; and completed TradeReturn changed mutable offer status, removing accepted agreements from memory. Actual Send→Arrival→TradeAccept→CourierReturn→Delivery→TradeReturn tests failed on the old code, then passed. The new archive now whitelists original offer terms pre-return and uses immutable acceptance membership. Physical source mutants reproduce both named failures and restore byte-exact. A rollback-only208→209 fixture also exposed completed historical rows missing their acceptance marker; returned (the production accepted-trade return state) is backfilled without inventing acceptance time. Declined/expired/cancelled rows remain unaccepted. CLI citation removal now honors --json on204 (named RED→GREEN).

## Migration rehearsal

Actual PostgreSQL15.19 live205 dump, retained privately outside git and backed up remotely; report `offline-pg15-upgrade.json` contains only aggregate counts/hashes and backup provenance. Owned offline15.19 rehearsal205→209→205→209 passed, all original worlds/settlements/goods/units/messengers data hashes identical, no legacy recipient invented. First attempt met Docker's temporary initialization server shutdown; readiness was changed to final TCP listener and the fresh second attempt passed. Failed log retained. No live migration/downgrade was performed by this rehearsal.

## Final validation and deployment

Final fresh suite, PG15 native K4/return-post, vet/JS and deployment provenance are pending. Existing pre-return trade-decision notifications/current-origin-owner seam is explicitly pre-existing and unchanged. Only archive visibility is claimed here. K5/K6/K7 remain outside this integration.
