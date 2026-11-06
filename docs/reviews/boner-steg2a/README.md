# Proof package — böner steg 2a: målstegen + starkare soldater + tåligare skrov

Order: `.agents/order-boner-steg2a-mal-styrka-skrov-20261106.md` · gren `boner-steg2a` från master `0cb09771` · migration **246** · ej pushad, ej mergad.
Inventering (steg 0): `inventory.md` i denna mapp.

## Slice-kontrakt (kort)
- **Problem:** en bön kan inte riktas; Velchanos har ingen stridsbön, Havsdaimonerna ingen skrovbön.
- **Spelarsanning:** spelaren väljer vem som välsignas (en egen enhet / stadens enheter / hela riket), ser priset per steg, och får verkan när den behövs (strid, storm) var enheten än är.
- **Invarianter:** bara egna enheter kan vara mål (ägarkontroll vid cast OCH läsning); staden = `support_settlement_id`; ingen `time.Now()`; events lagrar utfall.
- **Non-scope:** fart (2b), permanent (4), stormbönerna (3), Battle Frenzy-lagningen.

## Framgångskriterier mot test
| Kriterium | Test |
|---|---|
| Styrka ×1,25 per scope unit / city (via support, enhet på marsch) / realm | `combat: TestBlessedStrength_ScopesOwnerAndExpiry` (8 delfall) · `religion: TestLoadBlessed_ThreeScopes` |
| Oomfattad och främmande enhet får inget | `TestBlessedStrength_ScopesOwnerAndExpiry` (another city / foreign owner) · `TestLoadBlessed_OnlyOwnUnitsAndOnlyWhileItLasts` |
| Bön mitt i strid verkar från nästa runda | `TestBlessedStrength_MidBattleFromTheNextTick` |
| Efter `until_tick` / före `from_tick` ingen effekt | `ScopesOwnerAndExpiry` (expired, not yet cast) · `OnlyOwnUnits...` (99/100/109/110) |
| Skepp: en storm 0 skada, nästa full | `TestBlessedHull_TakesOneStormThenTheNextInFull` |
| City/realm ger varje skepp egen sköld; främmande skepp ingen | `TestBlessedHull_RealmWardsEachShipAndOnlyTheOwners` · `TestConsumeWard_OncePerShipPerBlessing` |
| "warded" slår ingen tärning | `TestBlessedHull_WardDrawsNoDie` |
| Storm före bönen täcks ej; Stillness-skyddad resa förbrukar ingen sköld | `TestBlessedHull_NotBackwardsAndNotWhenProtected` |
| Cast: kostnad ×1/×2/×4, rad, RiteCast-payload (target_scope, target_unit_id, blessing_id, until_tick) | `TestBlessingCast_CostScalesWithTheTargetAndWritesTheRow` |
| Fel ägare / fel slag / död / forming / utan mål → 4xx, ingenting dras | `TestBlessingCast_RefusesWrongTargets` |
| Samma mål två gånger 409; två olika mål OK | `TestBlessingCast_SameTargetTwiceIs409TwoTargetsAreFine` · `TestBlessingActive_SameTargetTwice` |
| Bredare mål kräver bredare gåva; längd mot eget steg | `TestBlessingCast_WiderTargetNeedsTheWiderGift` |
| Katalogen: mål och pris per steg | `TestBlessingCatalogue_ListsTargetsAndStepCosts` |
| Keryx: `--scope/--target` (namn), `--list`, fel före cast, warded-rad | `rite_target_test.go` (3 test) · `TestPrintSeaStormLine_WardedByStoutHulls` |
| Webb: målväljare, pris per steg, postar steg+enhet, warded-notis | `kult_offer.test.mjs` (4 nya) · `kult.test.mjs` (blessing prayer) · `format.test.mjs` (warded) |
| Codex: två artiklar + rites/storms/temples | `codex.test.mjs` · `TestCodexEntityCoverageFromRuntimeCatalogues` · `TestCodexReferenceMatchesAuthoritativeCatalogues` |

## Mutantlogg (committat före varje mutation; "dödad" = rött på en `_test.go:NN`-rad)
| # | Mutant | Dödad av |
|---|---|---|
| M1 | scope-filtret ignorerar `owner_id` (`blessingCovers`) | `blessing_test.go:100`, `:125`, `:157` |
| M2 | `BlessedStrengthFactor` 1,25 → 1,0 | `blessed_strength_test.go:108` (5 delfall), `:142` (testerna använder en literal 1.25, inte konstanten) |
| M3 | skölden förbrukas inte (`ConsumeWard` → `LoadBlessed` i sea_storm) | `blessed_hull_test.go:75`, `:104` |
| M4 | city-scope läser `settlement_id` i st f `support_settlement_id` | `blessing_test.go:104`, `:154`, `blessed_strength_test.go:108` (city-delfallet, enheten på marsch) |
| M5 | city kostar ×1 i st f ×2 | `rite_blessing_test.go:69`, `:191`, `:265` |
| M6 | målets ägare kontrolleras inte vid cast | `rite_blessing_test.go:134`, `:139` |
| M7 | skölden ignoreras (`if warded` → aldrig) | `blessed_hull_test.go:75`, `:104`, `:128`, `:145` |
Alla restaurerade (`git diff` ren efter varje körning). Rå logg togs i sessionens scratchpad.

## Ytor (ett verb på fyra)
- **temenos:** `POST .../rite` tar `target_scope` + `target`; `available_prayers[].targets` + `scope_cost_factors`; mig 246.
- **keryx:** `rite --scope unit|city|realm --target <namn|id>`; `rite --list` listar mål och pris per steg; storm-raden "warded".
- **webb:** Kult → Prayers → Offer: radioknappar (One named / This city's / The whole realm), enhetslista, fälten följer valt stegs gåva; storm-notisen "warded".
- **Codex:** `the-young-gods-arm`, `stout-hulls`; `rites`, `storms`, `temples`, `velchanos`, `sea-daimons`, `kharis`-tabellen (genererad).

## Vyer för BILD (Timothy dömer efter merge)
1. Kult → Prayers → *The Young God's Arm* → Offer: målväljaren med tre steg, priset (×1/×2/×4) och enhetslistan.
2. Samma för *Stout Hulls* med fartygslista ("in Knossos" / "sailing at (q,r)").
3. Storm-notisen "The Sea Daimons' blessing took the blow …".
(Inga skärmdumpar tagna i denna körning; BILD efter merge enligt arbetssättet.)

## Avvikelser mot ordern
- `wards_left` utelämnad (wards-tabellen bär all förbrukning, också scope unit); `from_tick` tillagd. Se `inventory.md` §5.
- `cast_event_id` är `bigint` (events.id är int8), satt i samma TX efter RiteCast-appenden.
- Kommentaren `sea_storm.go:54` ("never a ShipStormDamaged with 0") omskriven: ordern säger uttryckligen ShipStormDamaged + `warded` + `Damage:0`.
- Enheter i status `forming` kan inte väljas (inte byggda än); city/realm utan någon sådan enhet = 400.
- Pre-existerande `gofmt -l`-träffar (`api/handlers/unit_march_silver.go`, `world.go`, `cmd/keryx/cmd_codex.go`, `cmd_ticklog.go`) orörda.

## Svit
SVITRESULTAT
