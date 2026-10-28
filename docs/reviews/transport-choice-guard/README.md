# Direct transport choice guard

Contract: prove the dispatch backstop rejects an explicitly selected land route with a naval carrier. Scope is a transport integration test and proof; no production, migration or player-surface changes. This proves the trade portion of the geography→shortage→bronze→elite gate.

Use a real bound Galley, a valid sea lane, food and manifest. Call Dispatch directly, bypassing handlers which normally choose a matching category. The rejected order must return the choice/category error before creating a transport, manifest or arrival event, or loading food. A sea-selected control must dispatch successfully with the same fixture.

Physical mutation: neutralise only the explicit By/category guard in prepareJourney. The new named test must fail because land was allowed to dispatch as naval; restore exact bytes and verify green on another fresh DB. Full fresh suite is the final check. Claude owns review/integration; no merge/push/deploy by Codex.
