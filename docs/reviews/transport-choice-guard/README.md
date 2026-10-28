# Direct transport choice guard

Contract: prove the dispatch backstop rejects an explicitly selected land route with a naval carrier. Scope is a transport integration test and proof; no production, migration or player-surface changes. This proves the trade portion of the geography→shortage→bronze→elite gate.

Use a real bound Galley, a valid sea lane, food and manifest. Call Dispatch directly, bypassing handlers which normally choose a matching category. The rejected order must return the choice/category error before creating a transport, manifest or arrival event, or loading food. A sea-selected control must dispatch successfully with the same fixture.

Physical mutation: neutralise only the explicit By/category guard in prepareJourney. The new named test must fail because land was allowed to dispatch as naval; restore exact bytes and verify green on another fresh DB. Full fresh suite is the final check. Claude owns review/integration; no merge/push/deploy by Codex.

## Results

Core test commit `22316792`. Fresh migration220 direct test: green, including valid sea control. Neutralising the backstop physically produces the named assertion `explicit land choice dispatched a naval transport`; restoring exact production bytes gives fresh green. `mutation.json`, red/green logs and `run_mutant.py` reproduce this check. `go vet ./internal/transport` passed. Full `tools/gotest.sh` passed on fresh migration220 (world148.476s); `full-fresh-go.log` records the run at22316792.

No production behaviour or player text changed, so no visual/API surface update is required. Ship docking and the preserved river worktree are separate slices.
