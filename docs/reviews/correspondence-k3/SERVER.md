# K3 server evidence

Persistent drafts and private notes are keyed by the historical person and existing case. Saving sends nothing, creates no shared case and changes no messenger. An old tab receives409 rather than overwriting the newer aggregate.

## API and ownership

`GET /worlds/{worldID}/correspondence/cases/{caseID}/private` returns `{draft,note,version}`; a missing personal row is empty/version0 without creating a row. `PUT` requires `{draft,note,expected_version}` and returns the incremented version. Both require the original participant and at least one visible letter. A pre-delivery recipient, unrelated person, wrong world or nonexistent case gets404. Current city ownership is irrelevant.

Drafts use Send's existing1000-byte bound; notes have a technical10000-byte bound, both reinforced in migration208. Unknown fields, omitted/null fields, malformed/trailing JSON, negative or overflowing expected versions are rejected. Empty strings deliberately clear their fields. No private contents occur in Case/Cases, events or notifications.

A single SQL compare-and-swap covers creation and update. Concurrent first saves and concurrent existing saves each have one winner and one409. A conflict returns `current_version` with an instruction to retain local text and reload the saved state. It never silently merges or retries with a newer version. Draft and note share one version, so a note change also protects a newer draft.

The new table references case/person, never settlement/messenger. City takeover, loss of contact and city deletion do not hand over or erase private data. Removing the whole world/case removes its private rows. Original sender/recipient behavior and physical transport are unchanged.

## Tests

- Old K2 `f487f57c` on fresh migration207: `TestCorrespondencePrivateHTTPRouteContract` compiles and fails with HTTP404, expected empty version0. The test conditionally binds the production handler by reflection, avoiding a missing-Go-symbol failure. This is a chi-handler HTTP baseline; parent native acceptance verifies executable main routing.
- Fresh208 private suite: GREEN0.776s after all source mutations restored. Six named tests cover route contract, reload/person/case isolation, historical ownership/city deletion, validation/byte boundaries, absent and existing concurrent saves, stale versions, strangers/wrong world/pre-delivery access, and absence of shared case/messenger/event/notification side effects.
- Four physical production-source mutants individually produced the expected named behavioral failure: read the other person's row; bypass compare-and-swap; permit unrelated participant; permit undelivered case. Byte-exact restoration with matching SHA256 and precise intervals is in server-evidence/mutation-intervals.log. Final restoration:2026-10-24T06:44:59.983126+02:00.
- Reproducible runner: `tools/correspondence_k3_server_mutations.py`. Logs are under server-evidence. Mutation work occurred only in K3 after the parent's exclusive-build handshake; K2 was untouched.

## Scope and handover

Server files only; parent owns web/Keryx/Codex consumers, combined Go/JS/vet and native proof. No full-suite or native-browser success is claimed here. No merge/push/deploy. This delivers K3 private persistence, not K4 citations, K5 activity, K6 counteroffers or K7 warning aggregation.
