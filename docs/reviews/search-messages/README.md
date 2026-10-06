# Search: messages and rumours

## Contract (before implementation)
- Problem: Search advertises messages and rumours but never queries either.
- Player truth: search the current inbox and rumours already received, then open Correspondence or Gossip.
- Invariant: only authenticated existing player-scoped APIs supply results; no coordinates inferred from rumours, no undelivered letters or stale results from another search session.
- Scope: web search, its tests and Codex documentation. Existing server/CLI inbox and gossip semantics remain authoritative; no new verb.
- Non-scope: historical letter archive, sent letters, new API, graphical redesign, game rules.
- Acceptance: text/sender/region/category matches; click and Enter open the proper drawer; failures distinguished from empty results; reopening refreshes; existing map search preserved.
- Stop: any need to alter information visibility or game rules.
- Proof: existing JS baseline, failing regression before fix, full JS suite, browser interaction against deterministic API fixtures, served asset checks after static deployment.

Gate: proves access to information supporting geography → shortage → bronze → elite. FUNCTION + TEXT; existing search row styles, no BILD redesign.

## Evidence
- Baseline: all 6 pre-existing search tests passed at master `7772e2e6`.
- Regression first failed (new module absent); implementation passes all 367 JS tests, no skips.
- `python3 tools/search_messages_acceptance.py`: PASS using Chromium, real map DOM/search/fetchAuth and destination drawer renderers, explicit API fixtures. Tests case-insensitive input, HTML escaping, Enter/arrow/click navigation, refresh, partial HTTP failure, stale response ordering, and 390px mobile containment. Zero browser errors.
- Mutation: omit the `searchMessagesHTML` integration in search.js; browser test fails finding the copper letter. Restore: same browser scenario passes.
- Server contract inspected: Inbox in `messenger.go` returns only delivered letters to owned settlements (30 max); Gossip in `settlement.go` filters world + recipient (30 max). No new endpoint, domain verb, CLI or information rule.
- FUNCTION gate passed. Existing row styling reused; no visual redesign. TEXT: received-letter scope and navigation wording need ordinary playtest comprehension.
- No Go or DB change, so no Go suite or DB rig required. Browser API responses are fixtures, not proof of real-world message delivery. Clicking opens the existing drawer, not a selected conversation. Sent/replied-to letter history is outside this slice.

Run: `node --test $(rg --files web/static/js -g '*.test.mjs')` and `python3 tools/search_messages_acceptance.py`.
