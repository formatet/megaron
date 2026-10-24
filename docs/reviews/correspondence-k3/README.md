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

Implementation and verification in progress; no integration/deployment claimed.
