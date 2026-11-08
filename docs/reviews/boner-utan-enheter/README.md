# Proof: blessings on a city or realm without units

Order: `.agents/order-boner-utan-enheter-20261108.md`. Branch `boner-utan-enheter`. No migration.

## What changed
- temenos: `checkBlessingTarget` (settlement.go) no longer refuses city/realm scope when no unit of the kind exists; the count query is gone.
  `loadPrayerTargets` (cult_prayers.go) sets `Always: true`. Unit scope still needs a real unit (404/400); same prayer twice is still 409.
- web: `kult_offer.js` - for non-road prayers with `always`, city/realm text is "N supported by <city> now, and any that join while it lasts" /
  "any supported by <city> while it lasts" (and the realm equivalents). Road prayers (Swift Feet, Thread) keep their old text.
- keryx: `rite --list` prints the same for troop/ship blessings (`while the prayer lasts`).
- Codex: `rites.md` "Blessings with a target" says it; no article claimed the opposite.

## Tests (red before fix, green after)
- `rite_blessing_test.go`: `TestBlessingCast_CityAndRealmNeedNoUnitsAndCoverLaterOnes` (hull + strength, city + realm: 200, row written, a unit created afterwards is covered by `LoadBlessed`, repeat = 409), `TestBlessingCatalogue_CityAndRealmStepsAlwaysOpen`, unit scope with a missing ship = 404.
- `kult_offer.test.mjs`: two tests (N=0 and N>0 texts, steps enabled, no "nothing of this kind to bless").
- `rite_target_test.go`: `TestRiteList_TroopBlessingCityAndRealmHoldForLaterUnits`.

## Mutants (each reverted with git checkout)
- City/realm refusal restored: `rite_blessing_test.go:66`, `:166`, `:241` red.
- `Always: false`: `rite_blessing_test.go:204` red.
- Web N=0 realm text changed: `kult_offer.test.mjs:144/150` red.
- Keryx N=0 realm text changed: `rite_target_test.go:160` red.
