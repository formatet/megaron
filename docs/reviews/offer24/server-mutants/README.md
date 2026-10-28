# Offer deadline physical mutations

Checkpoint `738fb362`, isolated checkout. Run `python3 run.py CHECKOUT OUTPUT` with an isolated checkout. Every RED and restored GREEN run uses a fresh migration-218 PostgreSQL database and independently allocated free port. `summary.json` records assertion names and exact restored source hashes.

1. Restore new-offer lifetime to 168: actual Send buy/sell and resolved sea scheduling fail the literal arrival+24 contract.
2. Ignore saved expiry during arrival: an older 168-tick offer incorrectly gets a 24-tick bearer stay; the saved-deadline assertion fails.
3. Ignore saved expiry during passage resolution: replanning incorrectly changes the existing deadline and creates another timer; the immutable-deadline assertion fails.

All three gave named assertion failures, no compile failure/panic, then fresh GREEN after exact byte restoration. Production sources were never mutated in the parent worktree.

Policy checked: first unresolved new offers schedule arrival+24. Existing pending expiry timers, their deadlines and expires_at remain unchanged through passage replanning; arrival consumes the saved expiry for bearer stay. The shared resolver selects the earliest pending timer because that is the one the unchanged expiry handler will process first. No migration, notice change or event reinterpretation. Plain-letter stay is unchanged.

Other proof logs owned by the parent: baseline assertion RED `/tmp/offer24-baseline-red.log`; targeted fresh GREEN `/tmp/offer24-targeted-final.log`; full fresh messenger GREEN `/tmp/offer24-messenger-full-green.log`. Parent owns full Go and real-worker/native acceptance.
