# Inventory (2026-11-12)

Callers:
- `offerRowHTML`: `web/static/js/megaron/ui/drawers/kult.js` (okRite); ~30 asserts in `kult_offer.test.mjs`.
- `offerFields`: `kult_offer.js` (inside offerRowHTML); tests in `kult_offer.test.mjs`.
- `offeringFromEntries`: `kult.js` bringOffering; two tests in `kult_offer.test.mjs`.
- `scaledRecipe`, `scopeFactor`: `kult.js` (scope-change hook, bringOffering).

Prayers with a duration (`prayerBaseDuration`, server/api/handlers/cult_prayers.go): Walls (garrison_defense),
battle_frenzy, Green Shoots (harvest), Fruitful Season (fertility), Builder's Blessing (building).
Permanent step (scope_cost_factors.permanent): strength, hull, sea speed. Instant (duration 0): scapegoat,
walls fall, bird oracle, storm calm/wake, vision, horns, swift feet, thread. Prayers with chooser-kind targets
(storm/hex/city/place) have no scope steps; they are all instant.
Blessing prayers with scope targets and no base duration get a single "Gift:" line (their length is not stretched here).
