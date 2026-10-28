# K6 server contract

Problem: a counteroffer cannot currently replace a pending offer atomically.
Player truth: one command creates and dispatches a newly consented offer linked to the exact old offer; an unsuccessful command leaves the old one open.
Invariant: new escrow+bearer+case+audit and old decline+physical return commit together, or none do. The old sender learns no remote answer before physical return.
Scope: messenger Send/TradeCounter, correspondence action/projection, one unique JSON-link index, direct handler/PG tests.
No scope: K5 read receipts/presence, OfferExpired behavior, sea/risk/balance changes.
Acceptance: buy/sell linked replacements; escrow/refund on actual return; late failure rollback; auth/deadline/double/accept race; recipient privacy and actual later acceptance.
Stop: a new information exception or economic rule would be necessary. Proof: fresh PG target tests and physical mutants; parent owns four-surface acceptance/integration.

API: POST `/worlds/{worldID}/messengers/{messengerID}/trade-counter` with `{message,trade_offer}`. Returns normal Send201 receipt. Server binds old destination→old origin and its case. The Send request also accepts validated `counter_offer_id`; old/new settlement pair, original recipient authority and historic sender's current ownership are revalidated within Send's transaction. New terms own `trade_offer.counter_offer_id` (single JSON owner), with migration222 unique-expression backstop. Existing decline/refund_on_return handles old reservation physically. New event `TradeOfferCounteredV1` is an audit outcome, not an immediate counterparty notification.
