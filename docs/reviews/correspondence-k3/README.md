# K3 — persistent private draft and note

## Slice contract
Problem: a draft or private case note does not survive reload/device changes.
Player truth: each person saves their own case draft/note, sees whether it saved, and keeps a newer save when an old tab races it.
Invariant: counterpart/successor/stranger never reads or updates another person's private material; an unsent save creates no shared case or message.
Scope: per-person private persistence, GET/PUT private endpoint, optimistic version guard and consumers on four surfaces.
Non-scope: read/presence metadata, shared promises, citations, counteroffers, offline merge or implicit sending.
Acceptance: durable reload; two independent people/cases; stale and simultaneous edits conflict; historical participant retains data through city deletion; all unauthorized/pre-delivery paths fail.
Stop: no private save creates a shared case, infers historical recipient, sends a notification or changes a messenger.
Proof: real HTTP route404 on K2 baseline; fresh208 tests; named source mutants and byte-exact restored fresh tests; actual browser/CLI consumers in parent proof.

## Result and proof

Server `f79d588b`, clients `c6d35631`, migration208; dependent on K2 `ab6bb71c`. **Not merged or deployed.** [Server report](SERVER.md).

Baseline K2 realHTTP404 is named RED. Four server and four client source mutants are named RED, exact-byte restores GREEN. Fresh208 tests cover concurrent initial/existing writes (one success/one409), caller isolation, historical ownership/deletion, validation and absence of shared side effects. Client tests cover conflict retention, late-case responses and newer local edits while draft clearing is in flight.

Two independent owned PG/Redis worlds passed native Firefox1280/390 + actual Keryx. Process provenance: `76f4c64+k3/208/ok` and `c6d3563+k3/208/ok`; source hashes identical. Both labels explicitly include modified build state, not a claimed clean repository build. Normal register/join/found precedes labelled contact/delivery fixtures; actual letters and native private saves follow. Second browser page restores saved text, original saves newer, stale page409 preserves local text. Dispatch clears only sent draft; mobile retains private note. Actual CLI uses last-read version; counterpart reads its own empty private document. Historical city deletion leaves private text intact. No natural voyage/conquest is claimed. [Pictures](index.html).

762 JS tests and vet GREEN. Full fresh208 Go suite GREEN (world147.036s). No failed native run occurred in K3. The initial isolated JS harness lacked document and incorrectly expected a fetchAuth-only option at native fetch; both harness assertions were corrected before named mutations and full JS.

Limits: saving is explicit; unsaved edits are discarded when leaving a case, as stated beside the editor. There is one optimistic version for draft+note, not a merge algorithm. Only existing sent/delivered cases receive private documents; an unsent first letter creates no shared case. Citations and previous accepted transactions belong to K4; read/presence/motbud remain separate.

