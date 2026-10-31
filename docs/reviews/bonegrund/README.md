# Bönegrunden (bönesystem v3, slice 1) — bevis

Gren `claude/bonegrund-20261031`, migration 237.

**Problem.** En gudsbön hade fast längd, och vilan låg per gud och Wanax, så fler tempel gav ingenting.
**Lösning.** Gåvans storlek r (värde / traditionell baslinje, 1..5) styr längden på garnisonsförsvar och skördevälsignelse;
vilan ligger per tempel, växer med r, krymper med templets nivå, och räknas bara ned när templet är fullbemannat.

## Vad som ändrades
- `religion/cooldown.go`: `GiftFactor`, `DurationTicks`, `RestTicks`, `TempleRestRemaining`, `SetTempleRest`; `GodCooldownRemaining`/`SetGodCooldown` borttagna.
- `economy/temple_rest.go` `AdvanceTempleRest` (anropas ur `kharis/tick.go`): -1 per tick bara om fullbemannat, `last_tick`-vakt, rad bort vid 0.
- `economy/harvest_blessing.go`: `ApplyHarvestBlessingFor(…, ticks)`; `ApplyHarvestBlessing` oförändrad för gudomliga gåvor.
- `api/handlers/settlement.go` Rite: 409 per tempel med "rests for another N game days" + "stands still … (x of y servants)"; längd i svar och
  `RiteCast`-payload (`duration_ticks`, `rest_ticks`, `temple_building_id`, `gift_factor`); verdict-texten "more will not improve" ersatt.
- `api/handlers/cult_prayers.go` + `province.go`: `rest_remaining_game_days`, `rest_paused`, `temple_level`, `temple_staffed`, `temple_capacity`,
  `base_duration_game_days` (`cooldown_remaining_game_days` behålls = vilan, för äldre klienter).
- Migration 237: `temple_rest`, `DROP TABLE prayer_cooldowns`.
- keryx `rite --list` / `cult`: "Temple of Potnia rests 3 game days — paused: 2 of 4 servants"; `rite` visar längd före och vila efter.
- Webb: Kult-lådan (`kult.js` `templeRestText`), offerraden (`kult_offer.js`: bas-längd + "a larger gift lasts longer").
- Codex: `rites.md`, `temples.md`, `kharis.md`, fakta-raden i bönartiklarna; `tools/codex_facts.py` etikett; `articles.json` regenererad.

## Tester (rött utan ändringen)
- `server/api/handlers/temple_rest_test.go`: `TestGiftSetsPrayerLengthWithCap` (längd ∝ r, tak 5x, event + svar), `TestTempleRestShrinksWithLevelAndGrowsWithGift`
  (L1/L2/L3, gåva), `TestTempleRestRefusalNamesPauseUntilStaffed`, `TestAdvanceTempleRestOnlyWhenFullyStaffedAndIdempotent`.
- `server/api/handlers/religion_alpha_test.go` `TestAlphaPrayerTempleRestAndCertainAnswer`: samma tempel nekas, andra stadens Potnia-tempel kan be;
  `TestAlphaPrayerConcurrentCitiesAndOutcomeRollback` "same temple twice".
- `server/api/handlers/cult_test.go`: ett Asterion-tempel vilar (pausat), det andra är redo, fält i realm/city-paritet.
- `server/internal/kharis/temple_rest_tick_test.go`: kharis-tick räknar ned bara när fullbemannat.
- `server/internal/religion/gift_test.go`, `server/cmd/keryx/cmd_rite_test.go`, `web/.../kult.test.mjs`, `kult_offer.test.mjs`.

## Mutanter (committat före)
| Mutant | Dödades av |
|---|---|
| a. ta bort `/ nivå` | `temple_rest_test.go:85`, `gift_test.go:28` |
| b. räkna ned även undermannat | `temple_rest_test.go:127`, `temple_rest_tick_test.go:31` |
| c. ta bort `last_tick`-vakten (SELECT + UPDATE) | `temple_rest_test.go:141` |
| d. längd utan r | `temple_rest_test.go:54` |
| e. ta bort tick-anropet | `temple_rest_tick_test.go:38` |

## Kända avgränsningar
- Inga nya böner/mål; odds oförändrade; legacy-bönernas väntan (events) orörd.
- Pågående gud-väntan släpps vid migrationen (alfa, godtaget).
- r begränsas till 5 även för vilan (planen sa tak bara för längden).
- Äldre BILD-skript `tools/omen_dimma_bild_20261024.py`, `tools/byggplan_bild_20261024.py` raderar ur `prayer_cooldowns` och måste ändras om de körs igen.

## Sviter
`tools/gotest.sh ./...` (färsk DB, mig 237): 29 paket ok, 0 FAIL. `tools/test-web.sh`: 880 pass. `day_guard_test.py`, `test_verb_parity.py`, `codex_facts.py --check`: gröna.
