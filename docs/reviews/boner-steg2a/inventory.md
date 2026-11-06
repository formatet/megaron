# Inventering — böner steg 2a (Sonnet, 2026-11-06, master 0cb09771)

Inga kodändringar i detta steg. Fil:rad är läst i worktreen `boner-steg2a`.

## 1. Hur battle-rundan laddar deltagare
- `internal/combat/battle.go` `resolveTick` (~:454) laddar ALLA deltagare med en SQL (`battle_participants` JOIN `units`) i början av varje
  strids-**tick**, sedan körs `battleRoundsPerTick` rundor i samma TX utan ny laddning. Frysta vid inträde är bara `bp.divine` (kharis-handen).
  `defenseBoost` (Walls/Frenzy) läses redan färskt per tick via `settlements.garrison_defense_until_tick`.
- Alltså: en bön som bes mellan två strids-ticks syns från nästa tick (= "nästa runda" i den takt deltagare kan ändras; rundor inom en tick
  kan ingen bön avbryta, det är en TX). Krok: ladda `blessed` per deltagare efter radladdningen (en `religion.LoadBlessed` per
  deltagare) och läs det i `rollSide` bredvid `defenseBoost`/`divine`. `battleParticipant` får fältet `blessedStrength bool`.
- `rollSide` (~:1124): `diceCount` -> `defenseBoost`x1,25 -> hull-skalning (naval) -> gudarnas hand (slumpavrundad). Velchanos läggs
  direkt efter `defenseBoost`, med samma avrundning (`math.Round`), endast icke-naval (`unit.CategoryOf != naval`).

## 2. Hur sea_storm hittar skeppets ägare
- `seaVoyage` (`sea_storm.go`) bär `v.owner`, `v.shipID`. `sail()` låser skeppsraden `FOR UPDATE` (~:283) och slingrar `encounters`
  (~:337). Skyddet från Favourable Passage (`protected`) hoppar över hela slingan (~:322), så en `protected`-voyage förbrukar ingen sköld
  av sig själv.
- Krok: i slingan, FÖRE spare/wrath-tärningen: `religion.ConsumeWard(ctx, tx, v.owner, v.shipID, EffectBlessedHull, encounter.tick)`;
  true => `divine="warded", damage=0`, ingen `GodDice`-dragning. Förbrukningen skrivs i samma TX som skrovet.
- KROCK (löst enligt ordern): kommentaren `sea_storm.go:54` säger "never a ShipStormDamaged with 0". Ordern (beslut 7) säger uttryckligen
  `ShipStormDamaged` + `Divine:"warded"` + `Damage:0`. Följer ordern; kommentaren uppdateras. Webbens `notifText` och keryx
  `printSeaStormLine` skriver "battered by" / "keeps sailing" för Damaged — får en `warded`-gren. Codex `storms.md` får en rad.

## 3. PrayerActiveRemaining för målbönor
- Funktionen svarar per STAD (settlement-kolumner). För de två nya effekterna faller den på `ELSE 0` = inget globalt block. Det rätta
  blocket är PER MÅL: i Rite-handlern, efter målvalidering, `religion.BlessingConflict` (aktiv `prayer_blessings`-rad med samma effekt som
  täcker målet) -> 409 "already blessed". Katalogens `active_remaining_game_days` förblir 0 för dessa (katalogen har inget mål).
- Tempelvilan (`temple_rest`) är oförändrad och är det som i praktiken hindrar två olika mål i rad; testen rensar vilan mellan.

## 4. kult.js / bönerad
- `web/static/js/megaron/ui/drawers/kult.js`: `prayerStatusHTML` ritar knappen "Offer" (`okRite`), `okRite` öppnar `offerRowHTML`
  (offering-fälten) under `#offer-row-<id>`; `castRite` postar `{prayer, offering}`. `kult_realm.js` bygger kortet från
  `view.prayers` (flera `prayerRow` från `loadPrayerRows`). Målväljaren byggs i offer-raden (radio unit/city/realm + enhetslista) och
  `castRite` får `target_scope`/`target`.
- Keryx: `cmd_rite.go` har `--target` (UUID, "for future targeted prayers", skickas som `target`) och `listRealmPrayers`.

## 5. Avvikelser mot ordern att veta
- `wards_left` utelämnas ur `prayer_blessings`: tabellen `prayer_blessing_wards_used(blessing_id, unit_id)` bär ALL sköldförbrukning
  (också scope unit), så en andra räknare vore dubbel sanning. Lägger i stället `from_tick` (casttick) så att en storm FÖRE bönen aldrig
  skyddas av en bön som skannats i efterhand.
- Migration 246 var ledig (senaste är 245).
