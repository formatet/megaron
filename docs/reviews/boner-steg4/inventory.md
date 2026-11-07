# Inventering — böner steg 4, permanent på namngiven enhet (Sonnet, 2026-11-07, master 9b203f77)

Inga kodändringar i detta steg. Fil:rad läst i worktreen `boner-steg4`. Migration **250** (senaste 249).

## 0. STOPP-prövningen
Ingen läsare av skrovtaket ligger i en het slinga utan att redan ha skeppets rad i handen:
- strid: `battle.go:476` laddar alla deltagare med EN SQL per strids-tick (`u.hull` finns redan i den) — `u.hull_bonus` läggs i samma SELECT;
- storm: `sea_storm.go:~283` låser skeppsraden `FOR UPDATE` och läser `hull` — `hull_bonus` läggs i samma SELECT.
Ingen ny fråga per skepp. Inget STOPP.

## 1. Alla läsare av `HullMax` / `hullMax` / "5" som skrovtak
| Fil:rad | Vad | Blir per skepp? |
|---|---|---|
| `combat/battle.go:97,101` | konstanterna (basen, förblir 5) | nej, basen |
| `combat/battle.go:1160` `rollSide` | `hull/hullMax` skalar tärningar | JA: `hull/(hullMax+hullBonus)`; `battleParticipant.hullBonus` ur den befintliga deltagar-SQL:en |
| `combat/ship_hull.go:42-52` `hullLossForCasualtyFraction` | kvot -> hullpoäng, 0..5 | nej: poäng är absoluta (ett välsignat skepp tål samma slag, har en planka till) |
| `combat/ship_hull.go:59-70` `cargoSizeAfterHullLoss` | lastens förlust = förlust/max | JA: ny parameter `cap` (skeppets tak); annars nollas lasten när ett 6-skepp förlorat 5 av 6 |
| `combat/ship_hull.go:~235,252` `notifyShipDamaged` | `HullMax`-fältet i payload och notis | JA: `p.hullBonus` på deltagaren |
| `combat/sea_storm.go:368` | `HullMax` i `SeaStormPayload` | JA: `hull_bonus` ur skeppsraden som redan läses |
| `combat/ship_repair.go:152,173,183` | reparation klar: `hull = HullMax` | JA: `hull = $2 + hull_bonus RETURNING hull` |
| `combat/shipyard_work.go:64,79` | varvets färdigbygge: `hull = HullMax` när status `repairing` | JA: samma SQL-uttryck; nybyggda skepp rörs inte (hull DEFAULT 5, bonus 0) |
| `api/handlers/unit_repair.go:61,142,187` | "redan full", antal poäng, `hull_target` | JA: `combat.HullCap(u.HullBonus)`; `unit.Unit.HullBonus` laddas av `selectCols` |
| `api/handlers/storm_warning.go:18` | texten "Hull %d of 5" | JA: `Hull %d of %d` med skeppets tak; `stormShipWarning(type, hull)` får `cap` (callers: `trade_preview.go:179`, `province.go:3583`, `messenger_passage.go:228` — alla har skeppets rad/`unit.Unit`) |
| `api/handlers/unit.go:2348,2965` | enhetsvyn: `hull` | läggs `hull_max` + `blessings[]` (ägarens egen vy) |
| `web/static/js/megaron/ui/force_card.js:156-157,293` | `u.hull < 5`, `'/5'` | JA: `u.hull_max ?? 5` |
| `web/static/js/megaron/ui/format.js:658,673-676` | `body.hull_max ?? 5` | redan payload-driven; `ShipRepaired` (684) får `hull_max` |
| `cmd/keryx/cmd_hex.go:126`, `cmd_unit.go:396,531-532` | `Hull %d/5`, `hull %d/5 damaged` | JA: `HullMax` ur svaret |
| `cmd/keryx/cmd_notifications.go:612-674` | `hull_max` ur payload | redan payload-driven; `:706` "repaired (hull %d/5)" -> payload `hull_max` |
| `cmd/keryx/cmd_goods.go:434`, `cmd_storms.go:19` | "of 5" i hjälptext om stormen | text om basen; lämnas (skriver "of 5" för ett vanligt skepp) |
| `db/migrations/130_units_hull.up.sql:11` | `CHECK (hull BETWEEN 0 AND 5)` | JA: måste lättas, annars kan ett 6-skepp inte lagras. Mig 250: `CHECK (hull BETWEEN 0 AND 5+hull_bonus)` |

Orörda och riktiga som de är: `sea_storm.go` skadan (`StormDamage` absolut), `intercept.go:350`, `passage.go:1023`, `carrier.go:80` (hull>0, ej tak).

## 2. Hur permanent lagras och läses
- **Styrka / fart:** en `prayer_blessings`-rad, `scope='unit'`, `until_tick = 2147483647` (int4-max) och `permanent boolean`.
  Motivering: `blessingCovers`, `BlessingActive`, `ConsumeWard`, `SpeedBlessingFor` och `CoveredMovers` testar redan `until_tick > $tick` — en sentinel
  gör dem alla korrekta utan specialfall; `permanent` finns bara för visning och för spärrarna. Alla `until_tick`-läsare är Go-`int` (64 bitar) eller jämförelser.
- **Skrov:** ingen rad. `units.hull_bonus` (0/1) bär verkan; "redan välsignad" = `hull_bonus = 1`.
- Enheten dör: `disbanded` raderar inte raden (FK ON DELETE CASCADE gäller bara riktig radering), men raden är oläsbar eftersom enheten aldrig är levande igen.

## 3. Ägarens enhetsvy hämtar sin data
`api/handlers/unit.go` `unitSummary` (rad ~2330, byggs ~3010) -> webbens `force_card.js` och keryx `cmd_unit.go` (`army`/`unit`) och `cmd_hex.go:126` (egen enhet i hex).
Främmande Wanaxer ser enheter via `foreign_units.go`/`cmd_sightings` — de har inget skrovfält och får ingen välsignelse.

## 4. Flaggor / avvikelser från ordern
1. Ordern säger `battle.go rollSide, hull/hullMax`; det finns också `cargoSizeAfterHullLoss` som delar med `hullMax` — den får skeppets tak (inte i ordern; nödvändigt, se tabellen).
2. CHECK-villkoret på `units.hull` (mig 130) saknas i ordern och måste ändras.
3. Stout Hulls tillfälligt (sköld) på ett permanent skepp tillåts (ordern kräver 409 bara för styrka/fart).
4. Hull-bonus kräver ingen `prayer_blessings`-rad; ordern nämner raden bara för styrka.
