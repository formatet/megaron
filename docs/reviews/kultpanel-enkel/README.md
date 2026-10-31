# Kultpanel enkel (2026-10-31)

Beslut Timothy 2026-10-31: panelerna visar etiketter, tal och knappar; varför/hur står i Codex. Gren `claude/kultpanel-enkel-20261031`.

## Ströks / före → efter
- Lawagetas-briefs (misc.js `LAWAGETAS_BRIEFS`, `showLawagatasBrief`, `dismissBrief`, `briefDismissed`, localStorage `lb_dismissed_*`, CSS `.lawagetas-brief/.lb-*` i map.html, anrop i main.js): bort. Drawer-`?` (statiska knappar i map.html) stannar.
- "How the gods weigh gifts" (kult_values.js + test): bort ur Prayers.
- "≥5 kharis — requirement, not spent" → "Required kharis: 5"; "Traditional offering:" → "Offering:".
- "No cities yet. Found a city…" → "No cities"; "No temples — kharis will not climb…" → "No temples".
- Tempelrad: "Alternatives" och "Weight W · gives G … % of tithe" bort; "grain: paid · silver: missing" stannar.
- kult_kharis: demand/given → "Demand: N · Given: X"; upkeepformeln → "Upkeep: −x.x / game day"; tak → "Above ceiling: N"; arbetare → "Per extra temple worker: +x.xx"; blessing → "Blessings from kharis N"; wrath → "Wrath risk below N: 40% / game day, 80% below M"; idle → "Temple workplaces unfilled".
- Standing: visar `kharis_standing.short` ("Kharis 5.0 · Indifferent"), inte prosan.
- kult_offer: "X loves … The traditional offering is always enough." → "Favours: grain, oil"; "Choose goods…the traditional gift is unavailable" bort; kult.js behåller "Gift unavailable".
- Stannar: Temple workplaces unfilled, No temples, mat-varningen i kult_offer, bönegrundens vila/paus.

## Server / keryx
- `kharis.Standing.Short` (nytt JSON-fält `short`); `summary` oförändrad. Test: internal/kharis/standing_test.go TestStandingShortIsBandAndNumberOnly.
- keryx cult/status/rite/me skriver `short`; samma korta etiketter i status; cult: "Required kharis: N · Offering: …"; prayerPaymentText → "Required kharis: N".

## Codex
- rites (offerings: värdena är levande, listas ej; `keryx cult`/`rite --list` skriver dem; "Favours:"), kharis (per extra worker, above ceiling, upkeep, demand/given), temples (unfilled, Demand/Given). `?` per flik: index.json `drawer: "kult:prayers"` = rites, `"kult:temples"` = temples, kharis = kult; `registerDrawerTab` i codex.js.

## Tester
- notif_simple.test.mjs (briefs borta, `?` finns), kult_realm.test.mjs:27-47 (nya), kult_kharis/kult_offer/kult.test.mjs (uppdaterade), codex.test.mjs (`?` per flik), keryx cmd_status_kharis_test / cmd_rite_test / kharis_cap_line_test uppdaterade.
- Mutant: "requirement, not spent" tillbaka i kult_realm.js dödas av kult_realm.test.mjs:30.
