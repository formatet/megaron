# AI round1/2 player-surface fixes — slice5

Problem: sixteen observed preview, CLI and dispatch surfaces mislead players or hide the next action.
Player truth: forecast, status and order previews read the same domain rules and give usable commands.
Invariant: no balance changes, no new mechanics, no hidden first-leg coordinates/ETA exposed, old persisted events remain readable.
Scope: order points1–16, four surfaces where applicable; tests, physical mutations, native player checks.
Non-scope: inland balance (slice6), GPT player accounts until Timothy's configs exist, changes to the live world.
Acceptance: every point has a fix or evidenced reason; shared owner rather than duplicate validation; no absurd horizons/negative zero; reversible previews; full fresh Go/vet/JS.
Stop: canon conflict to Claude; branch+hash+STOPP, Claude integrates.
Proof: read-only Tylissos/Phaistos DB baseline; focused regressions/mutations; isolated API/CLI/web runtime and full fresh suite.
Gate: blocks geography→food/materials→bronze through misleading or absent player surfaces.


## Result

Source commits: `88308182` (core), `a1c8724e` (additional regressions), `997320cc` (city negative-zero text), `497c16f8` (HTTP effect expectations), `cd1e8c85` (stronger food-advice/overview tests only). Baseline master `81fe09d3`. No balance, production, growth, or live-world changes. Claude reviews/merges/deploys; hash + STOPP follows validation.

| Order | Resolution and proof |
|---|---|
| 1 | Forecast runway uses full FoodNet and suppresses floating-point dust ≤1e-9, without changing the physical meal/growth gates. CLI/web say “about even” where one decimal would produce signed zero; growth text handles the same case. `TestFoodRunwayTicks_NoDustInfinity`, `TestAIRound_NearZeroForecast`, JS forecast/growth regression; physical mutations. |
| 2 | Read-only live DB confirms Tylissos farm L1 at(58,54), not a missing gift. Server publishes `farm_possible` and `starter_farm_included` from the actual ChooseFarmHex/geography rule; clients no longer infer impossible farmland by comparing the same already-farmed forecast twice. HTTP + CLI/web regressions, natural inland founding with actual farm. Inland balance is slice6. |
| 3 | Status shows grain+fish−full consumption, taking authoritative food net from the server's shared summary; grain separately says production. Economy overview uses the same full net. No “move food workers” surplus advice while authoritative food balance ≤0. Real status/fixture and native coastal checks. |
| 4 | New dispatch reads transactional world state: join-count waiting line only in forming. Persisted old dispatches unchanged. Real authenticated active-world join regression + physical state mutant. |
| 5 | Goods header “Per day”. Native CLI output. |
| 6 | Missing world auto-detects exactly one active world and saves it with unknown config keys intact; ambiguous worlds get explicit `worlds --select <id>`. Explicit selection validates list membership and saves province context. Command/HTTP-stub regressions plus natural config without world_id. Login uses the same world resolver. |
| 7 | City lists both ordinal and(q,r); ordinal remains for place, coordinates for build --hex. No second ordinal parser. CLI regression + native city output. |
| 8 | New opening dispatch and founding Codex explain citizen placement on wood/stone ground and idle people producing nothing. Actual dispatch captured. |
| 9 | Explore preview runs combat.PreviewMarch, the same read-only domain validation used by dispatch, before returning the hidden-arrival response. Too-short and unreachable budgets reject. Successful validation discards first-leg details. Invalid-preview body/status matches actual order; no resources, jobs or unit state changed. Budget rejection uses the existing POST rejection text (which already mentions necessary travel budget), not a new hidden-route disclosure. |
| 10 | Noninteractive colonize without --yes explicitly says Nothing sent and does not POST; JSON also requires confirmation. Existing interactive refusal/rite cancellation already report cancellation. Founding's existing refusal now says Nothing sent. Host guidance occurs before colony text. Fake-HTTP command regression proves zero writes. |
| 11 | Structured goods shortages carry actual settlement_goods production rate; CLI/web show ceil(shortfall/rate) days only for positive rate, qualified “before other use”. Zero/absent rate gives no invented wait. Real HTTP stone shortfall, client regressions, physical rate-wire and CLI/web mutations. |
| 12 | Status grain line names grain held in Sitos. Overview adds per-good granary holdings from the same food summary, so Goods lists reserve alongside each good. DB parity fixture307grain, native real tithe/status/Goods. City liquid stock and granary stock remain separate. |
| 13 | Shared BuildEffectText/UpgradeEffectText says “this hex's stone/grain output ×…”. All existing engine/HTTP/CLI/web expectations aligned. |
| 14 | Own outward expedition says exploring toward unseen ground near its chosen area; no misleading first-leg destination as the order's goal. Existing expedition details retain turnaround/home. Location regression + physical area mutation. |
| 15 | Known preview says “Arrives in 3 days (world day42), if dispatched now”; same wording on web. Existing CLI read-only preview regression pins it. Hidden/courier arrivals remain unavailable. |
| 16 | Host colonize request/preview points at `founding settle`; never prints colony-consumption text. Ordinary colonists still see their irreversible-unit warning. CLI regression + physical Host-guard mutation. |

## Evidence and limits

`baseline-cli.log`: original production code prints signed rounded zero. `live-readonly-baseline.txt`: actual wall time, runtime health and read-only farm/rate rows; no live DB writes. No precise reproduction of an earlier tick's historical Phaistos allocation is claimed.

Nineteen physical mutations require named RED, exact byte restoration, then GREEN (`mutations.py`, `mutations.log`, `mutation-growth.log`, `mutations-food-extra.log`, per-arm logs). DB arms each create fresh PG16/migration181 with clean environment; CLI/JS arms do not inherit DATABASE_URL. One initial food-status mutant failed compilation due to an unused variable and was **not counted**; the corrected mutant preserves the variable and produces a named assertion failure. Initial fixture failures (active join had only one spawn tile; short expedition area was still close enough) were fixed as fixtures, not counted as product failures. The first full Go run found one remaining HTTP expectation for the intentionally changed hex-output wording; final full suite follows after that expectation commit.

Two separate natural own-port PG16/Redis/server rigs, no SQL fixture writes and no API response overrides: `native/evidence.json` runtime a1c8724e/181 and `native-repeat/evidence.json` runtime997320cc/181. Second run's Go production bytes are identical to final497c16f8; that final commit changes only a test expectation. Ordinary register/join → config auto-world → CLI founder forecast with explicit no-send → Host colonize guidance → normal CLI --yes founding → actual gift farm → actual Sitos tithe. CLI city/goods/status and original Firefox1280/390 forecast plus Goods captures saved alongside raw API/text evidence; screenshots inspected at1:1. Both runs zero browser errors. [Coastal Goods](native-repeat/coast-goods-firefox.png), [inland forecast mobile](native-repeat/inland-forecast-mobile.png).

The rig proves reachable forecast/config/founding/Sitos surfaces; short-expedition refusal is proven by authenticated real HTTP/DB preview-vs-dispatch test, not a claimed native long-distance journey. Tiny numerical boundaries use deterministic regressions, not chance map generation. TEXT remains for playtest; no new BILD style is introduced.

648 JavaScript tests passed, zero skips. Vet clean. Full fresh Go suite PASSED on migration181/source497c16f8 (world146.063s, `go-final.log`). The following cd1e8c85 commit only strengthens CLI/JS tests: the entire CLI package and JS suite were rerun GREEN after it; production Go bytes are unchanged. GPT player configs remain absent at the checked commit boundaries, so no GPT accounts were created or started.

Final status: FUNCTION checks GREEN, native repeat GREEN, no merge/push/deploy by Codex. Claude owns review and TEXT follow-up in playtest. World-selection parity added as pending review in megaron_verblista; own todo review row updated.


## Review follow-up contract — founding notification runway

Claude found that MetropolisFounded and ColonyFounded still divided by grain-only net outside the shared projection owner. Both must use economy.FoodRunwayTicks(stock, fullFoodNet), omit runway for floating-point dust and fish-covered meals, and carry additive full food balance for current CLI/web readers. Actual writer regression fixtures cover both notification paths; precise -1e-15 helper case and per-writer physical mutations; fresh full Go/vet/JS plus native notification reading. Existing persisted payloads remain readable. Same branch, new hash + STOPP.


Follow-up implementation: `5ce6c851`. Both writers call `economy.FoodRunwayTicks(grainAmount, foodNet)`; metropolis now carries additive `food_net_per_tick` and the same food warning as the colony. Existing grain balance and persisted alias names are retained; existing stored events are not rewritten. Current clients prefer full food net, and old grain-only payloads are described as grain production rather than a claim about all meals. Both current and legacy client paths suppress dust before rendering a runway.

`runway-baseline.log` proves the actual writers failed before the fix. The authenticated Settle test reads persisted MetropolisFounded from PostgreSQL; the real foundColony test records its broadcaster payload. Each covers (a) rate5−1e-15 against demand5 (binary net−8.881784197001252e−16), (b) grain0+fish5 against demand5, and (c) true deficit4−5. Dust/fish omit both grain_ticks and grain_days; genuine deficit retains stock/food-drain. Test-only PostgreSQL production triggers force those numerical boundary inputs, scoped to the fixture world and dropped in cleanup. They are deliberately integration fixtures, not natural gameplay proof. FoodRunwayTicks also has the precise standalone −1e-15 input.

Eight additional physical mutations (`runway-mutations.py`, `runway-mutations.log`) restore raw division or grain-only balance in each writer, and disable current/legacy dust guards in each client. All yield named RED → identical bytes → fresh GREEN. All649 JS tests pass without skips; vet-runway.log is clean. Full fresh Go suite PASSED on migration181/source5ce6c851 (`go-runway.log`; ok   formatet/megaron/server/internal/world 149.789s).

Natural third rig (`native-notice/`): runtime **5ce6c85173b5daebff1d1449ae726b24f1194b9c / migration181**. Normal inland and coastal founder flows plus persisted-notification API read and actual `keryx notifications`. Coastal MetropolisFounded has grain_net−4.5 but food_net+0.5, no grain_ticks/grain_days; CLI says food production covers consumption, net+0.5. Raw `coast-founded-notice.json` and `coast-closed-notification.txt` preserve it. No SQL writes or payload overrides in this native rig; zero browser errors. No naturally occurring −1e-15 outcome is claimed.

Review follow-up final status: all required checks GREEN; source5ce6c851; both writer mutations and both native persisted notification/API+CLI checks complete. New handover hash + STOPP, Claude integrates.
