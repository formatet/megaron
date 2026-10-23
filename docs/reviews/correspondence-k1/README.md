# Correspondence K1 — review package

Branch `codex/korrespondens-k1`; no merge, push or deploy by Codex. Delivered: server-owned incoming queue retaining unaffordable/expired offers, atomic late-response guards, default web Queue and read-only `inbox --queue`. Existing Correspondence loads only on selection.

Code: aa191904,90112d72,57173d91,f0e5ac57,ffd5bd51. Two final independent runtimes16206d2+k1/migration206/ok: [pictures A1–B4](index.html), [client details](CLIENTS.md), run1/proof.json and run2/proof.json. Normal register/join/found, then explicit35delivered-letter fixtures; real Keryx30+5, native Firefox1280/390 exact-open read/reply/decline, DB same-bearer return and one2copperrefund. No page errors/overflow. This proves client transactions, not natural delivery. Own runtimes removed.

Four named physical server failures: actual original late accept/decline, expiry removed, xmin membership, restore guard removed. Three client failures: wrong endpoint, invented accept, eager hidden history. All byte-exact restored and green. New-contract baseline delegated to unchanged Inbox and failed. Initial full fresh206 Go suite passed26tested packages(world154.706s). Review then found logical-restore xid hazard, requiring epoch fix and a final fresh full run:26tested packages, world148.225s (full-go.log). Final750JS/0 and vet green. Migration down/up preserves letters/owner; updates preserve insertion identity; imported future xids from old DB epochs remain visible, stale-process cursors fail closed. One read-only TX owns snapshot validation/read.

Limits: incoming per-letter queue with existing current-city-owner authority, not K2 historical private person archive. Stable chronological pages, grouping per page, no global urgency sort. No new presence/receipts/cases/notes/drafts/counteroffers/transport. Existing bootstrap/ws unread badge still uses legacy history fetch independently(K7). Shorter return-post window is separate follow-up.

Experiments retained: initial xmin snapshot failed after read updates; logical-restore review drove epoch guard. Browser attempt1 incorrectly counted desktop read in mobile; attempt2 used socket instead of TCP readiness; later instrumentation included pre-drawer badge history in Queue counts. Corrected TCP/context/request baselines; final two runs clean. See raw attempts and CLIENTS.md. None of these failed experiments is claimed as acceptance.

## Slice contract

Problem: pending offers disappear at expiry and the actionable inbox truncates at 30.
Player truth: delivered incoming letters and missed offers remain discoverable, with server-owned effective offer state and available actions.
Invariant: no undelivered letters, private counterpart read receipts, inferred presence or invented historical recipients; deadline checks and queue agree, including a delayed expiry worker.
Scope: GET /api/v1/worlds/{worldID}/messengers/queue, incoming per-messenger rows, existing recipient authority, keyset pagination with one insertion-membership migration, effective deadlines, Keryx/web/Codex consumers. Existing outgoing conversations remain available separately.
Non-scope: K2 person/case identities and permanent historical secrecy, outgoing status redesign, postponement/archive mutations, soon-warning thresholds, drafts, notes, presence, counteroffers, transport changes.
Acceptance: expired and unaffordable offers remain visible; equal-time rows traverse once with bounded pages despite new inserts; unread/read never changes offer action; recipient/world/withdrawn/delivery guards; late accept/decline cannot win against deadline.
Stop: a new information exception or unprovable historical recipient would be required.
Proof: actual DB HTTP contracts, named baseline and physical source mutation, targeted then one fresh full Go/JS/vet, two isolated browser/CLI flows.

## API contract

GET .../messengers/queue?limit=30&cursor=<opaque> (limit 1..100, default 30).
Returns {items:[], next_cursor?:string}. Each item retains Inbox fields (id, from_id/from_name,to_id/to_name,message,status,trade_offer,reply_text,sent_at,arrived_at,expires_at,read_at,affordable) plus queue_group (waiting_you/completed), offer_state (pending/expired/accepted/declined/cancelled or empty), actions ([] containing read, reply, trade_accept, trade_decline), and missing_payment when applicable. trade_accept means payment suffices, not a reservation of transport capacity: POST revalidates all domain conditions.
Pagination order is immutable sent_at DESC,id DESC within a captured upper boundary. States/actions remain live; clients group each page without claiming global urgency ordering. Cursors bind world/player, fail closed on malformed values, and survive state transitions without reordering rows. No total count promise. Rows inserted after the captured boundary appear on refresh, not halfway through an existing traversal.
A recorded expiry is labelled expired; pending with elapsed expires_at is also effectively expired while its worker is late, without mutating escrow/status on GET. NULL expires_at retains existing no-deadline semantics. Already-resolved statuses take precedence over a passed deadline. Read acknowledges a letter and never completes it. Free text awaits reply only while delivered. Historical received-letter proof and current settlement-owner permissions match existing Inbox; this is not K2's historical private person archive.

Membership uses a captured PostgreSQL snapshot and persisted insertion xid8 (migration206), never xmin: read/status UPDATE changes xmin and would otherwise drop unseen letters. Existing rows backfill in the migration transaction; every INSERT receives its creating transaction by default. Snapshot membership excludes new/backdated and initially in-flight inserts; live updates remain visible. No full-row historical snapshot is promised.

## Final insertion-clock regression

A final independent-clock review found that sent_at uses DB wall time whereas the game clock can lag after downtime. Using the game clock as the cursor's send-time upper bound hid otherwise delivered letters. The named fake-pause HTTP test failed on that code (paused-clock-before.log). Snapshot/DB-epoch/upper-bound now capture together using database clock_timestamp() for insertion bookkeeping, while effective offer state still uses injected game time. All Queue contracts plus time-language guard passed on a new206DB (final-clock-queue.log). The earlier26-package full run remains identified above; this final SQL capture correction received the focused checks that exercise it rather than another full run. Migration206 unchanged. Native proofs used ordinary aligned clocks; pause offset itself is proven through the actual HTTP handler/DB test, not claimed from the browser runs.
