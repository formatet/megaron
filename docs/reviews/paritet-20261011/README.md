# Browser parity — 2026-10-11

Base **4946d5ec**, branch **codex/paritet-20261011**, CSS/tool commit **a6aca06c**. Final result: **66 measurements, zero findings**, Firefox first, then Chromium and WebKit, at 1280×900 and 390×844. One production fix: `.inspect-row` gets `column-gap: .4rem` so a long river yield does not touch its label. No design decisions or gameplay changes.

Player truth: the changed map, city and Host information remains readable and its measured controls reachable at both widths. This slice checks the actual browser surfaces along geography → shortage → bronze → elite; the enriched city is a layout stress fixture, not proof of normal progression. Visual approval belongs to Timothy; screenshots are provided for that review.

## Findings and roots

| Finding | Evidence and root | Resolution |
| --- | --- | --- |
| Four Firefox “covered control” flags were false positives | [Baseline](baseline/findings.txt), [ancestor rectangles](clip-ancestors.json). A scrolled-away Slaughter button retained a positive viewport rectangle behind the city header, although its centre was outside the scrolling city body's clipping viewport. | The probe now checks ancestor clipping before testing whether a visible control is covered. Calibration still catches an actual overlay and an unreachable primary action. Removing the guard gives the named red `scrolled-away control wrongly counted as covered`. |
| River `Produces` label touched its long value | [Before rectangles](inspect-probe/rects.json): 0 px label/value gap in all three engines at both widths. `justify-content: space-between` cannot reserve a gap when the value consumes the row. | Added only `column-gap: .4rem` in `web/static/map.html`. Final gaps are about 6.8 px; wrapping remains natural. Physically removing the property makes the actual Firefox river panel test red: `river label and value touch`. |
| Intermittent startup resource DNS errors | [First corrected run](final-with-dns/findings.txt): 12 Chromium/WebKit startup flags. Chromium reported `ERR_NAME_NOT_RESOLVED`; WebKit named `umami.formatet.se`, the analytics script in `web/templates/base.html`. These were console resource errors, not application exceptions. | Reported as an environment finding; analytics is unchanged. A subsequent [three-engine startup probe](network-probe.json) had no console/page/request failures, and the final complete run also has none. Startup errors are now included rather than silently skipped by the parity probe. |

Both physical mutations are [named red](mutations.log), with [clip calibration](restored-probe.log) and [actual river checks](restored-fields.log) restored green. The production diff is one CSS property; all other changes are tools and proof.

## Coverage and result

Eleven surfaces per width and engine: map, river-focused map, city Production, city food/blessing, river hexpanel, Host with Details closed/open, and rich-city Production, food, Buildings and Garrison. Screenshots come from real login, `/play`, native desktop clicks and mobile taps. Map centring and read-only state imports select an actual known named river; they do not replace the renderer or fabricate the panel.

| Engine | Version | Measurements | Rich Buildings height, desktop / mobile | Built rows |
| --- | --- | --- | --- | --- |
| Firefox | 146.0.1 | 22 | 1422 / 1408 px | 30 / 30 |
| Chromium | 145.0.7632.6 | 22 | 1420 / 1407 px | 30 / 30 |
| WebKit | 26.0 | 22 | 1407 / 1394 px | 30 / 30 |

[Final measurements](final/measurements.json), [findings](final/findings.txt), [execution log](final.log). No horizontal page scrolling, out-of-viewport panels, measured text clipping, covered controls, inaccessible Host founding action, canvas backing-size mismatch or content-height divergence above the probe's existing 15% threshold. No browser console/page errors in the final run.

- Markraster runs in `worker` mode, with pending work zero at measurement. The map records live, remembered and fog tiers, three known rivers and one city for Eirene, at zoom 1. The river-focused view includes river labels, city icon and terrain raster.
- The actual selected river is **Sangarios (6,79)**: title **Delta of Sangarios (6,79)**, terrain **River Delta**. The terrain row does not repeat the name.
- The Host **Ships: never — inland** row is visible in both Details states and both widths. The founding action is still hit-testable.
- Mycenae and the rich city show the active blessing. The captured settlement API supplies `yield_bonus: 0.25`, remaining ticks and goods **grain, livestock, wine, oil**; the visible line uses days and excludes fish. Remaining days decrease while the rig ticks (Mycenae's first final capture has 1,856 days left).

Selected images, with all 66 retained under `final/`:

- Map/raster/river/city icon: [Firefox 1280](final/firefox-desktop-map-river.png), [Firefox 390](final/firefox-mobile-map-river.png), [WebKit 390](final/webkit-mobile-map-river.png).
- Mycenae blessing: [Firefox 1280](final/firefox-desktop-city-food.png), [Firefox 390](final/firefox-mobile-city-food.png), [WebKit 390](final/webkit-mobile-city-food.png).
- Host Ships: [Firefox 390](final/firefox-mobile-host.png), [Firefox Details 390](final/firefox-mobile-host-details.png), [WebKit 390](final/webkit-mobile-host.png).
- River spacing: [before Firefox 390](inspect-probe/firefox-mobile-river.png), [after Firefox 390](final/firefox-mobile-river.png), [after WebKit 390](final/webkit-mobile-river.png).
- Rich city: [Firefox Buildings 390](final/firefox-mobile-rich-buildings.png), [WebKit Buildings 390](final/webkit-mobile-rich-buildings.png), [Firefox Garrison 390](final/firefox-mobile-rich-garrison.png), [WebKit food 390](final/webkit-mobile-rich-food.png).

## Rig provenance and fixture

Isolated acceptance project `megaron-acc`, port 8097; no shared development deployment or database reset. [Final start provenance](provenance-final.txt): server **4946d5e**, migration **171/171**, world **3b5fb9cd**, tick **1158**, `TICK_SECONDS=6`, **2026-10-11 21:55**. Ticks continue during measurement.

The ordinary Docker build failed on Alpine package DNS ([log](build-rig.log); Docker progress carriage returns and trailing spaces are normalized in the build logs). The documented fallback rebuilt a CGO=0 Go binary from this worktree, copied current migrations and reused the cached acceptance runtime ([build](runtime-build.log), [binary/runtime hashes](runtime.json)). The health commit therefore describes the base Go code, which this slice does not change. Frontend files are mounted from this branch: [final served/local SHA checks](final-assets.json) include the changed `map.html`, map renderer, raster/worker, city icons and city production. This is proof of the current binary and branch assets, not a claim that the ordinary Docker build succeeded.

[Fixture facts](fixture.json), [SQL](fixture.sql), [settlement response](rich-city.json). Dedicated players ParityNomad and ParityRich were created/joined via normal APIs; ParityRich founded normally. Explicit SQL then added layout stress: 30 built rows, 12 named land cohorts, population/resources and a long-lived harvest blessing. The server's actual founded city is **Parity Rich Mycenae**, Minoan, Karpathia, coastal, at **(30,61)**. Maritime buildings are omitted to keep the measured 30-row fixture consistent. Garrison displays aggregate counts, not twelve individual cohort names. Mycenae also receives an explicit active blessing for this measurement.

The added workplace rows and resources are stress data; this does not establish that those rows are geographically producible through normal construction. Queues, expired blessings, other cultures, every zoom level and every game state are outside this slice. Mobile means a touch-enabled 390 px Playwright viewport; it is not a physical iOS/Safari test. Cross-engine pixel identity is not asserted.

## Validation and reproduction

All **568 JavaScript tests passed** ([log](full-js.log)); Python tools compiled; both mutations failed by name and restored green; final parity is 66/0. No Go production change, so no additional Go suite was needed for this CSS/tool slice.

From the branch worktree, with the existing acceptance world/account Eirene and the gitignored Compose `.env` present:

```sh
# Normally start the acceptance rig using its documented workflow.
# Only if Alpine DNS prevents that build, use the recorded local-build fallback:
python3 tools/parity_rig_runtime.py
python3 tools/parity_fixture.py
PANEL_USER=Eirene PANEL_HOST_USER=ParityNomad python3 tools/browser_parity.py --out docs/reviews/paritet-20261011/final --browsers firefox chromium webkit --surfaces map map-river city city-food river --rich-user ParityRich
python3 tools/parity_mutation.py
```

Fixture/runtime scripts write into this report directory and intentionally require the matching current server commit. After commits, rebuild the rig before rerunning them. A fresh world needs an owned-city player with a known named river in place of Eirene. Preserve the checked-in evidence by selecting a new output folder for a later browser measurement.
