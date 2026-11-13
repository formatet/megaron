# First hour, round two: fixes (branch forsta-timmen-varv2)

Base 7719ef54. One commit per item (`git log --oneline 7719ef54..`). No balance change: no cost, rate, place count or start store altered.
Rig: worktree's own `tools/acceptance.sh`, 4 players, one founded city (Teststad), Firefox (Playwright), 1280 and 390 wide. Pictures by `tools/varv2_bild.py`; measurement by `tools/varv2_knappar_matning.py`.

| Item | Reproduced? | Cause | Fix | Test | Picture |
|---|---|---|---|---|---|
| A timber/places | Yes: the hex rows said "N per game day" and nothing about places | `placement-options` carried `cap` but not which building raises it | server: `raised_by` + `extra_places` per hex good (`HexOption.RaisedByPerGood`, from the hex rules, `BuildingExtraPlaces`); web row "1 place here, 0 worked · a Lumbermill adds 4" | `TestRaisedByPerGood_NamesTheUnbuiltBuilding`, `placesNote` | A-timber-places-* |
| B no free workers | Yes (header only said "0 free") | none | "To move one, press −1 where they stand, then +1 here." beside "0 free" | `freeHint` | (text; not pictured: needs all 10 placed) |
| C buttons jump | Yes, measured: horizontal 20-120 px and vertical 26 px after one click, at 1280 and 390 | (1) the button box was content-sized flex-wrap, so a longer text on the left re-wrapped it; (2) the "Your citizens here" line appears with the first worker and pushed the rows down 26 px | fixed-width 3-column action grid; the workers line always reserves its row | `tools/varv2_knappar_matning.py` (exit 1 on a move > 1 px): all stable after | R-hex-labels-*, A-* |
| D choose a hex | Yes | `Choose a hex` ignored `payable`; a live drawer rebuild (WS) threw away the pick, tab and scroll | disabled like Build with the reason line; `loadCityDrawer` snapshots tab, scroll and pending pick (`currentPick()`) and restores them | build_afford BA3 (rewritten: it had pinned the old behaviour) | D-choose-a-hex-disabled-* |
| E stale stock | Yes by reading: nothing redrew the build rows between drawer loads; Economy re-fetches on open | no refresh path for ticks | the existing 30 s map poll calls `refreshCityStock`: one goods fetch, redraw only when a floored amount changed, scroll kept | not unit tested (DOM-heavy); not exercised on the page, see below | - |
| F food rows | Yes by code | per-good net coloured by sign | per-good food rows neutral; Food line red only when its net < 0 | warehouse test F (+2 old ones corrected) | F-warehouse-* |
| G keryx in web | Yes by grep (13 non-comment sites) | server sentences named keryx commands | server sentences surface-neutral (ship load, explore intent, pool empty, temple prayer, build hint, upkeep warning, capability hints); `refusalHint` in keryx adds the command back | `TestRefusalHintAddsTheTerminalCommand`, no-route test, small_texts | - |
| H explore | Yes by code | wording | "Nothing to explore there: everything within 5 hexes of (0,58) that these men can reach is already seen. Pick ground nearer the unknown." (radius from `ExpeditionAreaRadius`; web and keryx show the server sentence) | `TestNoReachableUnseenRefusalSpeaksLikeAPlayer` | - |
| I city picker | Yes | label | "Or pick a known city (optional)" in map.html and the Codex marching article (+ articles.json regenerated) | marchctx test I | - |
| J population | Yes | Walls hold / Citizens / People / In service scattered | lead row "Citizens N of M the walls hold" + "1 citizen = 100 people"; Free to work, Livestock, People, In service get glosses. "In service" = people drawn into forces (army units x per-man population cost, city.js `POP_COSTS`) | city_production tests | J-population-* |
| K naming | Yes by code: Enter did nothing, Name-it gave no visible change until the refresh | no key handler, no local result | Enter works; on success the note becomes "Named: <name> — cape"; refusals stay in the field's status line (`setErrorText`) | placename tests | - (needs a first-reach place; not reproducible in the rig) |
| L temples | Yes | rows had only a button | god's domain (from the four Codex articles) + cost and build time from the Construct catalogue entry for `temple` | kult_realm test L | L-kult-temples |
| M chips | Yes (also visible in L-kult-temples before the fix would have squeezed) | weighted shrink to 26 px | `chipLayout`: each chip keeps 250 px (about 18 characters of text with icon, time and x); the oldest hide and "x all (+n older)" counts them | chips tests | L-kult-temples (top bar) |
| N dispatch / join | Yes by code | the sentence is stored at join time; the join card counted settlements with an owner, not joined Wanaxes | sentence shown only while `WORLD_STATE === 'forming'`; join card counts `player_world_records` (same source as `wanaxes_joined`) | dispatch_window test N | - |
| O arrival | Yes | bare status word | server adds `home_city`; web and keryx: "X is home in Pylos (2, 59)"; old payloads: "— in the city" | format test O, keryx test | - |
| P no buildings/queue | Yes | empty-list text independent of the queue | "Nothing finished yet. Farm is being built, ready <time>." | city test P | - (stone is 0 in the rig, nothing to queue) |
| Q reload logout | Yes, at 1280 and 390 | the page cookie lived 15 minutes, the access token 24 h; navigation uses the cookie, API calls the localStorage token | cookie lifetime = `auth.AccessTokenTTL` | `TestTokenCookieLivesAsLongAsTheAccessToken` | - (verified: no cookie + valid token lands on `/`, with cookie stays) |
| R phone labels | Yes: 7.9 px / 6.1 px on screen (grid scale 0.87, also on desktop) | font-size in SVG units | at <= 480 px label 13 units (11.4 px), the "N goods" line dropped, a pick-mode step keeps a 10.5 px line | citygrid test R | R-hex-labels-* |

## Not tried / caveats
- E, K, P, B, G, H, I, N, O, Q fixes are test- or code-verified; not all have a picture (rig has 0 stone, no first-reached place, no queued build).
- Phone march-across-water forecast (G) was not driven on the page; the sentence change is covered by the combat and keryx tests.
- Item N: the "true number" is the world's start constant; the sentence does not say how many remain.
- Desktop hex-grid labels (7.9 px) were left as they are: the order named phone only.
- Capability hint `province_verbs.go:32` ("see `keryx build --list`") is an action description for `keryx actions` and was left.
- Pre-existing, unrelated: `TestCodexGrowthValues` (economy) fails on base too (growth.md number format); two handler tests (`TestShipyardDeployTransition...`, `TestWorldMe_OwnWorldIsolation`) fail only in the full-package run and pass alone, also on base.
