# Slice13b — ett matlager, Sitos bort

## Sammanfattning

Sitos är borttaget: staden har ett enda varulager som både armén och folket når.
Migration184 återför alla magasinsvaror utan klippning, även över cap eller utan befintlig goodsrad.
Köad gammal tick slutförs tyst, ingen ny Sitos-tick eller notis skapas. CLI/webb/Codex visar stadens
matstock, netto och tillväxt utan separat reserv. Historiska utfall och deras läsare behålls.

Kod **6975d36c**, operatörsverktyg/kommentarer **d571f27c**, extra faktiskt migration→upkeep-prov
**19b00caf**, gren `codex/ta-bort-sitos`, bas **6fccfe37**. Claude granskar/integrerar; **hash+STOPP**.
Ingen merge/push/deploy eller liveomräkning av Codex. Nästa14→15→16 väntar respektive START.

Proveniens: **ny läsande live-dump**, privat återställd i egen PG16, migrationsbas183;
[dump-SHA och alla före/efter-rader](snapshot-conservation.json), [SQL-/recompute-recept](snapshot_migration.py).
[source-health.json](source-health.json) är separat läsande healthz efter dumpen, inte samma tidpunkt.
Naturlig användarrigg: egen PG16/Redis/server, vanliga register/join/founding/placement-anrop,
[native/evidence.json](native/evidence.json) bevisar **6975d36c/184**, noll browserfel.
Ingen acceptance-compose användes; lokala recept och faktisk serverhealth identifierar mätningen.
Privat dump, configs, JWT-hemlighet och binärer committas inte.

## Slice-kontrakt

| Del | Kontrakt |
|---|---|
| Blockerar | Separat otillgänglig mat bryter försörjning längs geografi→brist→brons→elit. |
| Invariant | Stadens lazy stock + reserv perstad/pergood bevaras inom floatprecision; ingen ändrad armékostnad, fysisk skeppsproviant eller silverprojektion. |
| Mekanik | Ett settlement_goods-lager; ingen tithe/release eller ny Sitos-notis. Legacy ScheduledSitosTick no-op/avbokad, aldrig saknad handler/deadletter. |
| Fyra ytor | API foodöversikt, CLI/webb utan reserv och deltan; sitos.md och länkar bort. Historiska events ligger kvar med fryst betydelse. |
| Acceptans | Semantisk masterRED, migration på färsk privat kopia, övercap/missing-good/recompute, riktig upkeep och retry, CLI/Firefox, mutation/färsk Go/vet/JS. |
| Utanför | Frånvaronotiser14, textfynd15, rivning16 och nya logistik-/kostnadsregler. |

## Baslinje och mätning

[baseline.log](baseline.log): **6fccfe37/183**, verklig ny PG;
`TestFoodStore_SingleInventory` namngivetRED eftersom separat tabell finns. Inget kompileringsfel.

På den nya livedumpen bevarar184 **alla18 grain/fish-värden för nio städer**. Tabellen nedan visar
före som stad+reserv, efter som nytt stadslager (avrundad presentation; fullprecision i JSON).

| Stad | Korn före (stad+reserv) | Korn efter | Fisk före (stad+reserv) | Fisk efter |
|---|---:|---:|---:|---:|
| Amnissos | 174.236248+925.201611 | 1099.437858 | 282.801611+215.698389 | 498.500000 |
| Itanos | 788.107009+405.000000 | 1193.107009 | 0.000000+0.000000 | 0.000000 |
| Kydonia | 4.131017+36.754417 | 40.885433 | 271.824417+64.345583 | 336.170000 |
| Mallia | 507.444046+405.000000 | 912.444046 | 0.000000+0.000000 | 0.000000 |
| MalliaWest | 0.000000+0.318398 | 0.318398 | 29.243422+16.953181 | 46.196602 |
| Phaistos | 0.000000+441.811873 | 441.811873 | 2416.964169+472.288127 | 2889.252295 |
| Scheria | 0.000000+326.866757 | 326.866757 | 215.662500+34.333243 | 249.995743 |
| Thalassa | 0.000000+0.256552 | 0.256552 | 28.207412+13.241036 | 41.448448 |
| Tylissos | 52.984800+213.755554 | 266.740354 | 0.000000+0.000000 | 0.000000 |

Efter riktig **recompute-all på samma privata klon**: nio städer,0ändrade,0fel; samtliga18stockvärden
fortfarande identiska ([recompute-logg](snapshot-conservation.recompute.log)). Silverprojektionens hash och
historiska eventsantal är oförändrade av migrationen. Båda gamla tabeller saknas och ingen SitosTick är pending.
Ingen server/tickworker kördes på klonen. Testet är inte en lång framtidsprognos.

## Experiment och implementation

| Fråga | Slutligt svar/bevis |
|---|---|
| Hur flyttas stock utan dubbel lazy-intjäning? | Settle vid stadens egen world.current_tick, skriv amount/rate/calc_tick tillsammans. Ekvivalent med settled() i aktiva världen, korrekt även för arkiverade världar. |
| Förloras återförd mat över cap? | Cap höjs till max(befintlig cap, återförd totalsumma). Givet grain7+tvåticks×3+reserv11 blir amount24/cap24 trots gammalcap5; saknadfishrad blir17. |
| Klipps den vid nästa omräkning? | **Nej.** goodCap ger default endast för INSERT. RecomputeProduction ONCONFLICT bevarar befintligcap och klampar mot den; bygg färdig/placering/recompute-all använder samma funktion. Övercap-fixturen överlever riktig recompute: grain24/fish17/silver99. Ingen warehousemekanik finns i nuvarande kod. |
| När taket nås senare? | Befintlig framtida cap-regel gäller: ny överskottsproduktion kan gå förlorad. Migrationen ger inget oändligt lager och sänker inget tak efteråt. |
| Vad händer med gamla köposter? | Migration markerar oavslutade SitosTick klara och rensar failed_at; registrerad RetiredSitosTick accepterar sen leverans utan nya utfall/omschemaläggning. Normal dailyseeding innehåller inte typen. |
| Vad händer vid erövring/plundring? | Gamla extra läs-/debiteringsbenet mot magasinstabellen borttaget. Befintlig settlement_goods-loot är den enda vägen och samma varufilter/fysisk plundringskaravan gäller. |
| Config och gammal historik? | SitosConfig och dess tidigare oanvända konstruktorparametrar bort; SITOS_* läses inte längre. Gamla events/notiser återges fortfarande historiskt, utan nya utskick. |

`settlement_granary` och `processed_sitos_ticks` droppas efter återföringen. Down-migrationen återställer
bara tomma gamla tabeller; den flyttar inte tillbaka mat, sänker inte tak eller återöppnar gammal kö.
Ingen retroaktiv uppdelning eller omskrivning av historiska händelser görs.

**Livscykel:** `TestFoodStore_SurplusAndRetiredTick` kör riktig FoodTick/retry med all mat direkt i staden,
utan nya magasinsevents. `TestFoodStore_MigratedGrainFeedsGarrison` kör faktiska184-SQL följt av riktig
UpkeepHandler/retry: stadskorn0 + reserv10 blir tillgängligt, garnison100 oförändrad, noll attrition,
slutstock10−enmåltid (9,5), aldrig dubbeldebiterad. [migrated-army.log](migrated-army.log).

## Grindar

| Grind/yta | Bevis |
|---|---|
| Kod | Full ny PG16/migration184-Go på6975d36c: [full-go.log](full-go.log), world149,982s; [vet.log](vet.log) tom/grön. Efteråt endast operatörsverktyg/kommentarer och ett nytt test19b00caf; detta har egen ny PG-körning ovan. |
| Funktion/migration | [lifecycle.log](lifecycle.log), [recompute-conservation.log](recompute-conservation.log): övercap/lazy/missing-good, FoodTick/retry, legacyworker, seeding, API-paritet, tidigare svältsignal, retention. |
| Mutation | **7** fysiska kompilerande namedRED→byteexaktrestore→nyPG-GREEN: återfördamount, cap, missingrow, retiredhandler, reseed, API-nyckel och gammal plundringsfråga. [mutations.log](mutations.log), [recept](mutations.py), enskilda red/restored-loggar. |
| API | Province/overview delar samma FoodSummary och publicerar food{coverage_ticks,food_net_per_tick,growth_state}. Sitos/reservtrösklar/deltan utgår. Grain/fish, automatic_food och army_upkeep/net_after_upkeep behålls. |
| CLI | Status/foodnet/tillväxt läser food; Food lasts anger faktisk city grain+fish-stock. Ingen Sitos-/cap-/tithe-/release-rad. [native/inland-status.txt](native/inland-status.txt). |
| Webb | Stad/ekonomi läser samma API, More→Food details visar stock/folkmat/armémat separat. Firefox1280/390, inland/kust, byggarinspekterat1:1. [mobil](native/coast-city-food-mobile.png), [desktop](native/inland-city-food-firefox.png). Ingen ny bildfamilj; TEXT-begriplighet återstår i speltest. |
| Codex/JS | sitos.md/indexpost bort; alla länkar och gammal notishjälp pekar tillfood. Belägring använder citystock. **647JS,0fail** [js.log](js.log); historisk renderare och notiskind-mappning behålls. |
| Användare | [native.py](native.py): vanlig registrering/join/grundning inland+kust, verkliga CLI-anrop och placeringar, API-ekvivalens, Firefox1280/390,0page_errors. Inga SQL-writes, responseoverrides eller livekonton. |
| Drift | Verktyg använder food och nya DOM-id; SQL mot droppadtabell bort. Ändrade Python/JSON syntaktiskt verifierade, inte livekörda. CSV med borttagna kolumner kräver ny utdatakatalog. Claude äger deploy, healthz184 och operatörskontroll. |

[Remaining-references](remaining-references.txt) klassificeras som: gammal köidentifierare/no-op och dess vakter;
frysta event-/notisrenderare/arkivtest; faktiska migrations-/regressionstest; historiska CSV-eventräknare;
arkiverade migrationer och bevisrapporter. Ingen aktiv reservskrivare/tabelläsare/config kvar.
`snapshot.py` behåller historiska CSV-alias, `alfa_economy.py` historiska eventdelta,
`text_live.py` syntetisk arkivreplay; `lagersvangning_matning.py analyze` är uttryckligen äldre modell.

Första nativeförsöket fastnade i **riggens** gamla väntan på texten “in Sitos”; produktens nya text fanns.
Predikatet ändrades till befintliga “Food after army”, sedan kördes hela scenariot rent på ny värld.
[native-first-harness.log](native-first-harness.log) bevaras; [native-run.log](native-run.log) är slutkörningen.
Mutationsreceptet stoppade först före no-reseed-mutationen eftersom ankaret matchade två platser;
ankaret gjordes specifikt för dailyseed-listan och resterande tre fall gick namedRED→GREEN.
[mutation-harness-first.log](mutation-harness-first.log) är rålogg, slutresultatet är sju kvitton i mutations.log.

## Metodisk lärdom

- Konservera flyttad stock genom nästa riktiga skrivare, inte bara migrationens sista SELECT.
- En borttagning måste omfatta konstruktorer, kö/retry, plundring och operatörsverktyg utöver UI-raden.
- Frysta historiska läsare är kompatibilitet, inte en aktiv spelmekanik.

## Kända avgränsningar

Ingen lång balansprognos eller ersättning av gamla falska svältauditer. Frånvaronotiser och timestamps väntar14.
Ingen ny lager-/warehousefunktion; gammal capregel gäller framtida produktion. API-konsumenter måste uppdateras
med servern från sitos tillfood; repo-klienterna ändras tillsammans. Historiska rapporter/CSV-filer skrivs inte om.
Ingen merge/push/deploy eller produktionsdatabasändring av Codex.
