# K2 server proof

New letters create an explicit two-person case in the departure transaction. Historical recipient identities are not guessed. Archive snapshots survive city rename, ownership change and settlement/messenger deletion. Existing live recipient endpoints also enforce new-letter privacy; legacy operational access remains unchanged.

## Contract and lifecycle

- `GET /worlds/{worldID}/correspondence/cases?q=&limit=&cursor=` returns person-owned case summaries, immutable creation order, latest visible preview, letter count and cursor. `legacy_identity: unavailable` explicitly excludes unprovable old recipients.
- `GET /worlds/{worldID}/correspondence/cases/{caseID}?limit=&cursor=` returns visible letters, historical person/city IDs and names, original text and physically visible reply/outcome.
- Existing Send/SendFromHost accept optional `subject` (default Correspondence, max120 characters) and `case_id`. New cases are created by departure; existing-case append verifies both original people against the current destination owner. Both return `case_id`. Failed append rolls back messenger, escrow and scheduled arrival.
- Migration207 separates retained cases/letters from live messenger/city FKs. World deletion removes its archive; city/messenger deletion does not. Names use public `wanax_name`, falling back to existing public username behavior. No coordinates or current contact are granted by archive IDs.
- Delivery records receipt only on a physical delivered transition while the destination still belongs to the frozen intended recipient, holding a shared ownership lock. A successor receives no letter/archive/notification. A later ownership restoration or unrelated outcome update cannot invent an earlier receipt. The physical bearer lifecycle remains unchanged.
- Validly delivered history stays with its historical recipient even after city loss. Sender reply text and remote trade decisions remain hidden until return. Sender's own cancellation remains visible/searchable immediately. No exact read time, activity, motive, or new FOW rule is published.
- Cursor membership uses creation transaction snapshot and database-process epoch. Ordering uses immutable case creation or letter send keys; activity previews do not reorder cases. Equal timestamps, late backdating and append-during-pagination are tested. Cursors bind world, person, case, search and epoch.

## Production-write inventory

`rg INSERT INTO messengers` across production Go found three user-message insert statements: settlement Send's trade/plain branches and SendFromHost, all in messenger.go; both public departure functions now attach the archive in their transaction. `api/handlers/unit.go` inserts physical command runners (`kind=order`) and intentionally creates no correspondence case. Legacy migrations/test fixtures create no inferred historical recipient. Reply reuses the same messenger/case; all worker/accept/decline/cancel/fate outcomes persist through one narrow status/trade/reply update trigger. No existing event meaning changes. New inbox/queue/read/reply/accept/decline checks require the frozen recipient; settlement outbox/cancel require the original sender for new correspondence. Host outbox already keys directly by sender. Arrival notification additionally checks the same receipt authority.

## Evidence

- Old fa83fc93/migration206, unchanged real Send flow: `TestSendCreatesPersistentCaseIdentity` compiles and fails because successful send has no case_id. See server-evidence/baseline-missing-case-id.log.
- Fresh207 targeted cases/reply/host and physical arrival notification regression: GREEN. Four physical source mutants individually produced named behavioral failures, then exact byte restoration and fresh207 GREEN (handlers2.602s, messenger0.042s). See server-evidence/* and tools/correspondence_k2_server_mutations.py. Every mutation/restore has timestamp and matching SHA256 in mutation-intervals.log.
- The first successor mutant survived because a manually delivered test letter lacked the worker's durable arrival event; this independently hid returning history. The fixture now includes that event, and the repeated mutant fails on leaked inbox content. Original survivor output retained; no green claim is made for it.
- Additional fresh207 pagination/cursor/remote-trade-search tests GREEN (handlers0.385s). The own-cancellation regression was added afterward and is included in the parent's combined final run; no earlier run is claimed to cover it.
- Parent owns final combined Go/JS/vet/browser/CLI verification and integration. No server-subagent merge, push or deploy.

## Limits

This is K2's archive substrate, not K3 drafts/notes, K4 citations, K5 presence, K6 counteroffers or K7 warning aggregation. Old operational inboxes retain their previous ownership model; no historical-private-archive claim is made for rows whose recipient cannot be proven. A letter reaching a city after recipient ownership changed is deliberately not a private receipt for either person; no new routing/return mechanic is invented.
