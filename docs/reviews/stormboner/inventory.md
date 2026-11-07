# Inventering — stormbönerna (3b), master 49c1283d, före ändring

Beslut: krympningen KAN skrivas säkert mot en samtidig väderscanning (inget STOPP). Skäl nedan.

## 1. "Hex på din karta" (väck stormen)
`/map` (world.go:~413) låter en hex vara känd om `province.AnyEyeSees(eyes, pos, terrain)` (live-ögon: `loadLiveEyes`) ELLER hexen står i
`loadRememberedTiles` (player_scouted_tiles + scoutade provinser + kontaktade städer). Samma test finns som `knownToPlayer(eyes, remembered, pos, terrain)`
(world.go:1202), använt av trade_preview/province/colonize. Väck återanvänder `knownToPlayer` + `province.LoadTileGraph` för terrängen. Aldrig en hex som är svart.

## 2. Hur krympning och återväxt går in i väderskanningen
- Skanningen (`stepWeather`, sea_storm.go:549) tar `pg_advisory_xact_lock(hashtext("sea_storms:"+world))`, sedan `ensureStorms`+`advanceStorms` i EN TX och
  läser varje storms form ur `sea_storm_track` (`latestStormShape`). Bönen tar SAMMA lås i sin TX (`CalmStorm`/`WakeStorm`), så de serialiseras: antingen ser
  skanningen bönens ändrade rad, eller bönen ser skanningens färdiga rad. Ingen hål-läsning, ingen förlorad uppdatering.
- **Krympning:** bönen skriver om storms SENASTE spårrad (tick `last`): tar bort den valda hexen och numrerar om slots 0..n-1. Nästa skanningstick utgår
  därför från formen med en hex färre, och spårraden för nästa tick har en hex färre. Sen skanning = dagliga: bönen sker på en definierad plats i
  ordningen (efter skanning till `last`, före nästa), och skanningen är tick-för-tick, så den ser samma form oavsett om den kommer sent.
- **Återväxt:** tabell `sea_storm_calms` (mig 249). Skanningen läser väntande rader (`regrown_tick IS NULL`) och, i tick-slingan för en storm som ska stega,
  försöker lägga till en hex INNAN stegen vid `t >= regrow_tick`: en havsgranne till stormen som ingen levande storm upptar (upptaget = alla levande
  stormars form vid tickens början), kandidaterna sorterade (q,r), tärningen väljer. Ingen kandidat -> ingen tärning, försök igen nästa tick. Lyckad
  återväxt sätter `regrown_tick = t` i samma TX. Allt sker i tick-slingan, så en sen skanning ger samma utfall som dagliga.
- **0 hex = död:** stormen med 1 hex som stillas får `dies_tick = died_tick = last+1` (första tick utan spår); spåret före står kvar (som 3a:s död),
  skanningen laddar bara `died_tick IS NULL`. `/storms` och omen filtrerar redan på `died_tick IS NULL`.
- **Fiske:** `economy/stormHexesNow` läser spåret för nuvarande tick. Efter en krympning räknas städer med fiskare på den bortagna hexen om i bönens TX
  (annars hade hexens fiske legat blockerat: nästa skanning letar bara hexar som FINNS i spåret). Vid död släpper skanningens `recomputeStormFishing`
  (spårraden `last` finns kvar och ligger i dess tick-fönster).
- **Väck:** `WakeStorm` tar låset, läser alla levande stormars senaste form som upptaget, kräver att målet är havshex och fritt, bygger formen med
  `seedShapeAt` (utbruten ur `seedShape`: samma tärningsordning), `insertStorm` med `dies_tick = tick + stormLife`. Skanningen hoppar en storm för `t < created_tick`
  (ny kolumn i `liveStorm`), så en väckt storm inte upptar hexar i ticks före sin födelse.

## 3. Paketgränsen
`api/handlers` får importera `combat` (cult_prayers.go gör det redan); `religion` importerar INTE `combat` (G1). Alltså: tärning, form och tabeller i `combat`
(`storm_prayers.go`), böne-specarna i `religion/prayers.go` (bara konstanter/specar), målvalidering och Rite-grenen i `api/handlers/settlement.go`.
`religion.IsBlessing` rör inte de nya bönerna (de är inga blessings): egna effekttyper `storm_calm` / `storm_wake`.

## 4. Avvikelser från ordern (att granska)
1. **Idempotensnyckel = `cast_key uuid`** (genereras i handlern, finns i `RiteCast`-payloaden), inte `cast_event_id`: effekten körs FÖRE att eventet har ett id
   (samma problem som `prayer_blessings.cast_event_id`, som länkas efteråt). Unikt index på `cast_key`; samma nyckel två gånger = en effekt.
2. **Väck lagras i egen tabell `sea_storm_wakes(cast_key,…)`** (ingen kolumn på `sea_storms`, så att ingen läsare av stormraden kan skilja en väckt storm).
3. **Krympningen skriver om senaste spårraden** i stället för en rad för "nästa tick" (en rad för nästa tick vore en förhandsskrivning som skanningen skulle ha att
   undvika). Nästa tick får ändå en hex färre. Beskrivet ovan.
4. **Måltypen i Rite-requesten:** `target` = stormens namn (stilla) resp. `"q,r"` (väck). Katalogen får `storms[]` resp. `wake: true` under `targets`.
5. **Bönerna är "everyday" (har GodID)** och svarar alltid; ingen tärning på att guden svarar, bara på VILKEN hex som tas.
