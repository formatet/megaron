# S1/H1: placement rates and horses

## Contract
Problem: the chariot article mentions horses without explaining whether they are a managed good.
Player truth: chariots require a Stable and bronze; horses are not currently a managed good.
Invariant: no recruitment, economy or catalogue mechanics change.
Scope: Stable/Chariot prose and generated article bundle; inspect existing placement text.
Non-scope: future horse breeding, terrain gates and storage.
Acceptance: both articles clarify horses; S1 checked in web/Keryx; bundle and Codex checks pass.
Stop: any discrepancy with the approved H1 rule in Temenos_varutaxonomi_sol.
Proof: existing Codex suite, bundle consistency and deployed asset comparison. TEXT comprehension remains a playtest question.
Gate: proves the geography → shortage → bronze → elite chain through clearer recruitment requirements.

## Baseline
Master 2e550aca; live healthz: {"commit":"2e550ac","migration":228,"status":"ok"}.
Existing Codex suite: 27/27 passing. S1 already implemented: citygrid.goodRowHTML says
"per game day next"; Keryx goodCell uses rate(), which says "per game day".
The H1 clarification is absent from both articles. No new verb or server change is needed.

## Result and checks
Stable and Chariot now explicitly explain H1. The existing recruitment owner
`server/internal/province/training.go` requires Stable and charges bronze, with no horse cost.
27/27 Codex tests pass after the edit; `tools/codex_facts.py --check` and `git diff --check` pass.
Mutation: not applicable to prose-only changes; no executable guard or algorithm changed.
Visual layout unchanged; comprehension is TEXT for playtest, not claimed as human-tested.
No Go production changes or database changes; a full database test run is not relevant here.
Live baseline ground counts: 16 players, 7 settlements, 24 units; migration 228.

## Deployment
Production checkout: `45b2aa982c66525845badbdb5bd7e27077751721`.
HTTP-served stable.md, war-chariot.md and articles.json exactly match the committed bytes.
The unchanged Go process reports `2e550ac`, health ok, migration 228 clean; static files
are served from disk and need no restart. Ground counts remain 16 players, 7 settlements,
24 units. Todo S1/H1 closed; H1 comprehension retained for playtest.
