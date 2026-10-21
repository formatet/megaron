# Replacing an unboarded movement order

## Summary

When a Wanax dispatches a new movement order, an older order Runner waiting unboarded in an owned port could still deliver its obsolete order later. The dispatch now withdraws such older orders in the same database transaction and sends their Runners home through the existing physical call-back path. This branch is ready for Claude's review; it has not been merged or deployed. Base: `4995f608`, migration 195.

## Slice contract

| Field | Contract |
| --- | --- |
| Problem | An older Runner at an owned port can deliver a superseded movement order after a new order is sent. |
| Player truth | The dispatch receipt says how many older orders were withdrawn; each old Runner walks home, and the new order still travels. |
| Invariant | Withdrawal and replacement commit together; no Runner aboard a carrier, in a foreign port, or carrying an independent stance order is withdrawn. |
| Scope | Messenger call-back owner, unit-order dispatch, API response, Keryx and web receipts, Codex marching article. |
| Non-scope | Runners already travelling on land or aboard a ship; changing delivered-order precedence. |
| Acceptance | A new movement order returns the older Runner physically; rollback leaves the earlier order untouched; aboard and stance Runners remain outbound; the four player surfaces agree. |
| Stop condition | Any need to change the interpretation of existing events or recall a Runner from a foreign port. |
| Proof | Fresh database domain and API tests, full Go suite, web suite, vet, and an API mutation. |

## Baseline and experiment

The branch started from an uncommitted WIP on `a7151d3d`; it was rebased onto `4995f608` before final verification. The earlier reported domain-test hang did not reproduce: the candidate query now closes `pgx.Rows` before invoking call-back queries. The domain test passed on a fresh database before and after the rebase. Its failure paths now roll back their transaction before `Fatalf`, so a red assertion cannot leave a lock behind.

| Hypothesis | Change | Result | Decision |
| --- | --- | --- | --- |
| A new dispatch must own withdrawal and replacement together. | `SupersedeUnboardedMovementTx` calls `CallBackTx` in the existing dispatch transaction. | API test verifies old Runner returning, new Runner outbound, and one scheduled physical return. | Keep. |
| The API test detects a missing withdrawal. | Temporarily replaced the supersession call with an empty result. | Fresh migration 195: failed at `superseded=[]`; restored code passed on another fresh database. | Keep test and implementation. |
| A different unit's waiting order must survive. | Added a second owned unit with its own waiting Runner. Replaced the unit-id predicate with `$3<>''` as a mutation. | Fresh migration 195: named domain test failed with three candidates instead of two. Restored predicate: domain and API tests green on a separate fresh database. | Keep the unit-id guard. |

## Gates

| Gate | Result |
| --- | --- |
| Code | `tools/gotest.sh`: all Go packages green on fresh migration 195, including `internal/world` (148.417 s). `go vet ./api/handlers ./internal/messenger ./cmd/keryx` green. |
| Visual | No layout or graphics change; receipts are text additions. |
| Semantic | Receipt reports withdrawal at dispatch, without claiming the new order has arrived. Codex distinguishes waiting, aboard, and other travelling Runners. |
| User | HTTP route test exercises authenticated dispatch through the router and checks response plus database state. Keryx and web receipt text have focused tests. |
| Client | All 692 `web/static` Node tests green. |
| Drift | Not deployed; Claude owns merge and deploy. |

## Known limit

This slice intentionally leaves older Runners that have left the port or boarded a carrier on their existing routes. Their orders still obey the current delivered-order precedence. No live-world orders were changed.
