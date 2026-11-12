# Prayers: the gift's size as fixed steps (web, Codex, keryx help)

Branch bon-gavosteg from master a04d690b. Order: `.agents/order-bon-gavosteg-20261112.md`. Not merged, pushed or deployed.
No server rule code changed. Inventory: [inventory.md](inventory.md).

## Premise
`TestGiftStepsGiveNTimesTheLength` (`server/internal/religion/gift_test.go`): for every everyday prayer and every scope except
permanent, N x recipe x scope factor (N = 1, 2, 3, 5) gives r = N and length base x N exactly. **Caveat found:** r carries float
noise (3.0000000000000004), and `RestTicks` uses ceil, so the rest can be one tick longer than N x (e.g. 37 not 36 at N=3).
Test allows +1 tick for rest; the engine is untouched. Worth a separate small order (round r to 1e-9 before ceil) if it matters.

## What changed
- `kult_offer.js`: `GIFT_STEPS`, `giftSteps`, `checkedStep`, `giftStepsHTML` (pure); `offerRowHTML` now shows steps (radios, first payable
  checked), one "Gift:" line for instant prayers and "For ever", and the number fields inside a closed `<details>` "Offer other goods"
  (open at once when the city lacks the recipe). The fields always mirror the checked step, so the existing `offeringFromEntries` path
  sends nothing for step 1 and the exact goods for step N; a field the player edits is what is sent. `chosenTarget` ignores step radios.
- `kult.js`: scope change re-prices the steps (keeps the chosen step if still payable); step change fills the fields; food line follows.
- `megaron.css`: `.offer-gifts`, `.offer-step` (min 44 px), `.offer-other`.
- Codex `rites.md` (lead bullet and the "Without the traditional gift" paragraph), `articles.json` regenerated; `keryx rite --offer` help gains the N-times sentence.
- Changed assertion: `kult_offer.test.mjs` "Bonegrund" test (old "Lasts 6 game days ... a larger gift lasts longer" note is now the step list).

## Clicks, "bless one unit for twice as long"
Before: pick unit scope, then edit every number field by hand (2+ fields, calculator needed to know the result) = about 5 actions and a guess.
After: Offer, choose the unit, check "12 game days", Bring the offering = 4 clicks, length and price on screen.

## Pictures (Firefox via Playwright, `tools/bon_gavosteg_bild.py`; real modules and CSS, fixture HTTP, readable names)
Desktop 1280 and mobile 390, each asserting no sideways scroll and steps >= 43 px high:
A all steps payable · B two steps unpayable (Phaistos) · C instant prayer · D1 city x3 chosen · D2 "For ever" · E "Offer other goods" open.

## Not tried
Against a live world or a real POST of step N (the request is the existing composed `offering`; premise proven only by the Go unit test, not the handler).
Storm/city/place chooser prayers were not pictured (they have no steps). keryx not rebuilt.
