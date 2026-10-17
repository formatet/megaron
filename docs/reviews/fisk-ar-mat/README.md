# Fisk är mat — slice 4

Problem: fish-fed settlements receive grain-only starving loyalty penalties.
Player truth: meals paid by the shared feeding engine count as meals for loyalty.
Invariant: FoodTick owns actual food shortfall; economy.FoodNet owns projected grain+fish balance. No third feeding formula, no change to stored event semantics.
Scope: welfare, founding forecast and additive colony payload; web/keryx consumers and Codex, regression/mutation evidence.
Non-scope: feeding order, growth, army rations, diet variety, kharis and artwork.
Acceptance: fish-fed is well fed, true uncovered need is starving, reserves and exact-last-meal work, forecast matches full food balance, all player surfaces agree.
Stop: incompatible canon/order ambiguity is sent to Claude; hash + STOPP, no integration by Codex.
Proof: fresh database FoodTick→welfare lifecycle, baseline RED, physical fish-exclusion mutation, full fresh Go/vet/JS and player-surface checks.
Gate: blocks founding→food→production chain by giving a false starvation signal.


## Result and ownership

Source commit `c963a055`, baseline `2bf95ac7`. Welfare reads `settlements.food_unmet_amount` written by FoodTick at priority55; welfare runs at70. It no longer re-derives hunger from grain rate/stock after a meal has consumed that stock. **The separate production-deficit loyalty penalty is removed while reserves still feed people**, explicitly approved by Claude (2026-10-17,14:02); Claude takes that clarification to Timothy. The actual starvation result still penalises welfare. Growth, feeding order, rations, variety and event types are unchanged.

Founding warnings use `economy.FoodNet` on grain+fish. They describe future shortage after reserves run out, rather than declaring an empty-grain fish-fed city hungry. The forecast's own balance now calls that same owner. Colony notifications gain additive `food_net_per_tick`; web and CLI prefer that full balance and retain readers for old grain payloads. No persisted event is rewritten. `citizens.md`, `founding.md` and `loyalty.md` explain the same rule.

## Evidence

- `baseline.log`: unmodified production code on fresh migration181; actual FoodTick feeds fish, old welfare nevertheless records starving. Grain/fish reserves and emergency livestock also reproduce the false penalty.
- `TestWelfareFoodTick_FishAndReserves`: real DB FoodTick→welfare, six cases: exactly5 fish/zero grain, combined shortage, grain reserve, fish reserve, livestock fallback, no food. Reads actual unmet, event reason/type and replayed loyalty points.
- `TestColonizePreview_FishFoodWarning`: authenticated real HTTP forecast with fish production and zero grain seed. `TestFoundingFoodWelfareWarning_MatchesFoodNet`: deficit, exact sufficiency, surplus. Browser/CLI rendering regressions cover fish-fed and shortage notifications and old payload reading.
- Six physical mutations, each named RED→identical source bytes→fresh GREEN: fish excluded from FoodTick, unmet ignored by welfare, fish excluded from forecast warning, warning disabled, full-food field ignored by web/CLI. Reproduce with `python3 docs/reviews/fisk-ar-mat/mutations.py`. Logs: `mutations.log`, `mutations-rest.log`, per-arm files. An initial mutation anchor had stale whitespace; this is recorded as harness failure, fixed before the four remaining successful arms.
- Natural own-port PG16/Redis/server rig, no SQL writes or API overrides: `native/evidence.json` carries runtime health **c963a05543f0bb8b0a7bf7aa7211167d6747f8b2 / migration181**. Normal register/join, third natural spawn: grain0.5+fish5.0−consumption5.0 = +0.5. Both CLI and Firefox1280/390 show the same full balance without false grain warning. Normal founding then actual `well_fed` loyalty event, zero browser errors. Original browser captures: [desktop](native/forecast-firefox.png), [mobile](native/forecast-firefox-mobile.png); raw API/CLI/web text and loyalty event beside them. Screens inspected at1:1.

## Validation and limits

644 JavaScript tests, zero skips; `go vet ./...` clean. Full fresh Go suite PASSED on migration181/source c963a055: `ok   formatet/megaron/server/internal/world 146.030s` (`go.log`). An exploratory nonstandard package order (loyalty→combat→handlers) failed two existing courier tests (`relevant-go.log`); this is not counted as the required full-suite result. Canonical fresh `tools/gotest.sh` runs packages in its normal order. The initial two-coastal-tile forecast fixture produced only4 fish and correctly warned; the final fixture adds a third real known coastal tile to exercise sufficient fish.

The natural founding carries its normal1200 grain: it proves reachable surfaces and live well-fed welfare, **not zero-grain fish-only consumption**. That boundary and actual shortage are established by the database lifecycle test. No native colony-march scenario is claimed; additive colony message rendering has regressions in both clients. No migration or integration/deploy by Codex. Claude owns review, merge and deploy.

Shared vault decision updated in `temenos_settlement.md` and indexed in `megaron_styrande_beslut.md`; pending-review status is explicit. This is FUNCTION plus TEXT; wording is judged in playtest, no new BILD gate.


Final status: all required checks GREEN; source c963a055; handover branch codex/fisk-ar-mat. Hash + STOPP, review/integration by Claude.
