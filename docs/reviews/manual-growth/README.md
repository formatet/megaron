# New citizens await placement

Contract: Timothy2026-10-20 wants each newly grown complete100-person citizen to
remain free for manual placement. Preserve founding's starting-food placement,
invisible remainder food and reinforcement's use of actual growth. Scope: growth
hook and affected kharis policy tests, citizens Codex. No existing placements
removed, new UI or demographic balance change. Proves player labour choice.
Acceptance: crossing199->200 grows one free citizen with unchanged placements;
founding/remainder/reinforcement still work. Baseline: fresh kharis suite before edits.

Fresh kharis baseline and final suite GREEN. The replay-idempotency fixture now
explicitly assigns its newly grown worker before testing replay, keeping its
food-positive premise without depending on automatic placement. Restoring the
old hook produces named RED in TestApplyDecay_NewWholeCitizenRemainsFree.
Founding and invisible remainder are unchanged; no existing placement is moved.
