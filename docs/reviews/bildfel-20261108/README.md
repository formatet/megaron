# Bildfel webb 2026-11-08 - proof

Order: `.agents/order-bildfel-webb-20261108.md`. Branch `bildfel-20261108`. Tests written first (red), then fixes; one commit per point.

| # | Fix | Commit |
|---|-----|--------|
| 1 | `routeCargoHTML` appends each blessing after the ETA | 616d22d7 |
| 2 | Walls Fall `Where` = `"X's, wall N when last seen"` (coordinate dropped from SQL and text); `.offer-target-city/.offer-target-place { max-width:100% }` | 1fce6faf |
| 3 | KeryxReleased link gets `class="notif-link"`; `a.notif-link` uses `--accent-city-text` + underline (rule is global, so it hits notis list, dispatch window and chips) | d7df8436 |
| 4 | `c.count === 1` -> "your one city" | 65fbc137 |
| 5 | `blessingsSectionHTML` (own "Blessings" dsec, empty when none) via placeholder `#city-blessings-sec`; harvest + storm rows stay under Food | eda77802 |

Point 5 note: the section is injected, not part of the static template, because test J asserts exactly three primary `.dsec`.

## Mutants (all restored, tree clean)
- blessings removed from routeCargoHTML: `economy.test.mjs:264` red
- `=== 1` -> `=== 2`: `kult_offer.test.mjs:288` red
- notif-link class removed: `placename.test.mjs:102` and `:109` red
- cityBlessingsHTML back in foodSummaryHTML: `city_production.test.mjs:161` red
- Go Where text changed: `walls_fall_prayer_test.go:278` red

## Suites
- tools/test-web.sh: pass 949, fail 0
- tools/gotest.sh ./api/handlers/... (fresh DB, mig 254): ok
- day_guard_test.py OK; codex.test.mjs 29/0; go build + vet clean
- Logs grepped for FAIL / not ok / _test.go: nothing outside the intended mutant runs.

## Not done / flagged
- No test for the CSS rules (no CSS test infra); BILD is Opus's after merge.
- keryx prints `where` verbatim (`cmd_rite.go:108`); its fixture still has the old text but asserts only "held by Agamemnon" from its own JSON, so untouched.
- Codex: no article quotes the changed texts.
