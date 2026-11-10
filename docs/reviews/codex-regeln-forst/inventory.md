# Inventory (before edits)

## How the lead (text before the first `## `) is used

- `codex.js` `splitSections` makes part 0 (id null) the intro; search (line ~279) indexes each part as a hit, intro included; the reader renders the intro above the contents list (line ~417 slices it off for sections).
- `cmd_codex.go` `codexSectionText(body, "")` returns the intro; `keryx codex` search results (line ~247) use it as the article text/snippet. The first sentence is the summary line.
- `codex.test.mjs`: heading anchor test (line 210) uses `splitSections().slice(1)`; no test constrains the lead today.
- Consequence: the first line of each lead must stay one sentence summary; anchors live only on `## `/`### ` headings (ids are matched with the `{#id}` suffix), so every heading that has an anchor must stay.

## Words and anchors

| article | words (wc -w) | cap 60 % | anchors |
|---|---|---|---|
| rites | 2934 | 1761 | rites-details, prayer-requirements, prayer-requirements-2, rites-the-prayers, minoan-prayers, minoan-prayers-2, minoan-prayers-3, offerings |
| runners | 2482 | 1490 | runners-details, runners-why-this-is-good-for-you, runners-when-an-order-fails, runners-catching-up-with-a-moving-unit, runners-catching-up-with-a-moving-unit-2, runners-crossing-the-sea, runners-the-runners-tab, runners-runner-speed, arrange-passage, arrange-passage-2, runner-lost-at-sea, runner-lost-at-sea-details, runner-lost-at-sea-details-2, runner-passage, runner-passage-2, runner-passage-3, runner-passage-on-map, runner-rescued-at-sea, runner-rescued-at-sea-details, runner-return, runner-waiting |
| kharis | 2078 | 1247 | kharis-numbers, kharis-details, kharis-details-2, raising-kharis, raising-kharis-2, raising-kharis-3, divine-wrath, divine-wrath-warning-before-danger, divine-wrath-seven-game-days-after-punishment, divine-wrath-temple-levels |
| ship-types | 1792 | 1076 | fetch-by-ship, fetch-by-ship-2, landing-in-own-city, landing-in-own-city-2, sailing-time, ship-food-return, ship-loss, ship-missions, ship-missions-2, ship-missions-3, ship-types-2 |
| marching | 1719 | 1032 | marching-giving-a-march-order, marching-giving-a-march-order-2, marching-giving-a-march-order-3, marching-a-marching-unit-is-still-reachable, marching-orders-to-forces-already-in-the-field-travel-by-runner, marching-orders-to-forces-already-in-the-field-travel-by-runner-2, marching-where-you-can-march, marching-on-the-road, marching-exploring-an-area, marching-exploring-an-area-2, marching-terrain-and-journey-time, marching-terrain-and-journey-time-2, marching-terrain-and-journey-time-3 |
| temples | 1638 | 983 | temples-building-one, temple-workers, daily-offerings, rich-offering, daily-offerings-2, temple-levels, temple-levels-2, temples-find-your-temples, cult-upkeep, cult-upkeep-2, temple-demolition |
| buildings | 1426 | 856 | buildings-details, buildings-details-3, hex-buildings, hex-buildings-2, hex-production-buildings, building-levels, building-levels-2, demolition, demolition-2 |
| units | 1413 | 848 | units-recruiting, units-recruiting-2, units-recruits-are-your-own-people, units-recruits-are-your-own-people-2, units-recruits-are-your-own-people-3, units-the-forces-tab, units-the-forces-tab-2, units-forces-and-marches |
| welcome | 312 | 188 | welcome-your-opening-dispatch, welcome-three-rules-that-explain-almost-everything, welcome-where-to-begin |
