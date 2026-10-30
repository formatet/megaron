# Religion v2 — inventory and contract

Order: `.agents/order-religion-r1r8-20261030.md`; approved values:
`megaron_religion_talforslag_20261030.md` in the vault.
Gate: proves growing realms can sustain the geography → bronze → elite chain.

## Contract before code
Problem: cult income and burden still use linear workers and population, with a flat indivisible gift.
Player truth: realm demand is cities²; each temple contributes level² × staffed fraction × paid tithe fraction.
Invariant: each tithe part is either fully paid or untouched, from that temple's city; no silver substitute for grain.
Scope: kharis/economy temple rules, transition, projections, API, Keryx, web and Codex.
Non-scope: R2 storm/battle/punishment probabilities, prayer design and horse/storage work.
Acceptance: 2/4/8 workplaces; 1/4/9 weights; squared demand; independent tithe parts; one notice and 14 ticks protection.
Stop: missing canonical values or conflicting contract.
Proof: fresh DB baseline, targeted regression/mutation tests, full Go/JS suite, user surfaces and live ground-state verification.

## Inventory
- `economy/temple_staffing.go`: capacity owner; consumed by staffing, placement/recompute and old staffing transition.
- `kharis/temple_offering.go`: shared read/charge plan, city-local shared stores in god order, row locks and all-or-nothing gift.
- `kharis/tick.go`: old worker coefficient, population decay, scarcity, maintenance and audit events.
- `kharis/project.go`: read projection must match actual maintenance; population-based breakdown and fed-worker reader.
- `kharis/wrath.go`, punishment path in `tick.go`: realm shield in kharis_notice_state; retain mechanism and max existing protection.
- `cmd/server/main.go`: startup transitions; transition must precede workers and retain a durable one-time marker.
- `api/handlers/cult.go`, `province.go`: realm/city kharis payload, temples and per-temple gift plans.
- `cmd/keryx/cmd_status.go`, `cmd_cult.go`, notifications: realm burden, worker hint, local gift totals and temple rows.
- `ui/drawers/kult_kharis.js`, cult tabs and temple gift rendering: same old burden/worker explanations.
- `internal/chronicle/chronicle.go`: maintenance event reader; preserve old meanings and add V2.
- Codex kharis/temple/offering articles, `reference.json`, generated `articles.json`, notification registry/index.
- Existing tests assert the old formula in decay/devotion/labor-flow/temple-staffing/offering/transition-notice/realm rules;
  update expected mechanics while preserving the tested lifecycle and transactional guarantees.

No production edits precede this inventory commit. Baseline master f33d1533, schema 228.
