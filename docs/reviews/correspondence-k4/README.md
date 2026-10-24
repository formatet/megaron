# K4 — accepted engine transactions and private exact citations

## Slice contract
Problem: past accepted terms and personally bookmarked passages cannot be consulted together for a known person.
Player truth: memory separates recorded accepted trades from exact passages marked My note, with stable links to source cases/letters.
Invariant: private citation never becomes a common obligation; the new archive never reveals remote acceptance/reply/transport knowledge before physical return; no guessed fulfillment or betrayal.
Scope: accepted-letter projection, person-scoped paginated memory, private exact citation create/delete and four consuming surfaces.
Non-scope: debt/reputation, free-text promise inference, new trade/transport rules, presence, counteroffers, notification aggregation.
Acceptance: cross-case same-person memory; only allowed accepted outcomes; exact visible message/reply quotes; owner-only persistence/deletion after city loss; stable pagination and retry dedup.
Stop: no absent delivery outcome is inferred; historical acceptance time remains unavailable where no timestamp was recorded.
Proof: old K3 HTTP404 baseline, fresh209 auth/visibility/lifecycle/cursor tests, named physical source mutants with restored tests, parent native/CLI proof.

Build complete; final verification below. No integration or deployment claimed.

## Result and proof

Server `a96b1474`, exact-source API `f119025e`, web/Keryx `be7d99ff`; migration209. Depends on final K3 `175c1ee2`. **Not merged or deployed.** [Server report](SERVER.md), [exact-source correction](SOURCE-LETTER.md).

Old K3 realHTTP404 is named RED. Six server + six client source mutants are named RED, restore byte-exact and GREEN. Fresh209 tests cover accepted/returned snapshot membership, two exhausted cursor streams, private source author, exact quote, retry dedup, city lifecycle and access isolation. A first client label mutant survived because a section heading matched a loose assertion; the actual paragraph label is now asserted and that same physical mutant fails. See `client/first-survivor.md`.

Two independent final owned PG/Redis worlds passed native Firefox1280/390 + actual Keryx with identical production source hashes. Process provenance: `055ed3d+k4/209/ok` (explicit modified build label) and **clean source build `be7d99f/209/ok`**. Normal register/join/found precedes labelled contact, delivery and stock fixtures. Actual HTTP sends a sell offer and accepts it; recipient memory records the accepted terms, sender archive memory excludes it until explicit physical-return stage. Native controls save/retry a private passage, then mobile removes it; actual CLI recreates/reads it. Counterpart sees no private note. Names/city deletion preserve memory. Native exact-source link opens the precise stored letter; final CLI proof does the same. No natural voyage/conquest is claimed. [Pictures](index.html).

768 JS tests and vet GREEN. Initial full fresh209 Go suite GREEN (world147.670s); the second full fresh209 suite after the final exact-source API/CLI change is GREEN (world144.474s). The first native pass exposed pale inherited h4 headings; contrast was corrected before both final passes. Native scripts needed no further failure correction.

Limits: delivery outcomes are explicitly unavailable in the memory record; no live remote transport joins or accusations are inferred. Memory is paginated across the same original person, not a new debt/reputation engine. **Pre-existing immediate OfferAccepted/OfferDeclined notifications remain outside this archive slice** (`messenger.go` TradeAccept/Decline, current originOwner). Only the new archive's return-gating is proved; no global absence of pre-return knowledge is claimed. The notification/current-owner seam needs separate reproduction/contract review before K5/K7. K5 read/presence, K6 atomic counteroffers and K7 notification aggregation are not implemented.
