# Gift destinations follow actual map knowledge

Contract: fix the observed leak of unknown foreign cities through the transfer
menu and execution endpoint. Both must consume existing map knowledge (live eyes,
remembered tiles and completed contact), not the obsolete radius6 halo. Preserve
own-city transfers, physical cargo and payment atomicity. Scope: gift listing and
POST, shared existing visibility reads, relevant tests/Codex. No new visibility
rules, messenger semantics or UI layout. Blocks geography/contact gate.
Acceptance: unknown city inside old halo is absent and cannot receive a gift;
known city is listed and accepts; refusal causes no debit/mover. Baseline: existing
gift tests fresh193 before edits. Stop if this requires changing the knowledge model.

Evidence so far: existing gift tests baseline GREEN on fresh193; new gift suite
GREEN. Unknown city at distance5 (inside old radius6, outside real sight) is
absent from GET and POST returns403 with unchanged silver and no transport.
Remembering that exact tile lists the city and POST201 succeeds. Both physical
old-gate controls independently produce named RED; restored files in finally.
Reproduce with `python3 tools/gift_knowledge_mutations.py OUT`. This fixes only
gifts; remaining letter/trade radius gates are separately recorded, not silently
changed. UI and CLI use these existing endpoints unchanged; Codex describes the
actual known-location requirement.
