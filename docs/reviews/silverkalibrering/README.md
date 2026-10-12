# Silverkalibrering — STOPP 1

**Status: inventering och talförslag; STOPP före produktion.** Gren `codex/silverkalibrering`, eget worktree `/home/tk/wt/codex-silverkalibrering`, bas `acf5c96f`. Inga server-, SQL-, webb-, Keryx- eller Codex-produktionsändringar. Claude beslutar om förslagen och ger separat implementationstillstånd.

**Slutsats:** standardproduktion ger 1 dagsverke/enhet för de flesta råvaror, fältsten 2 och brons **11 inklusive 9 koppar + 1 tenn + gjutaren**. Silvrets beslutade kulleankare ger 1. Dev-kartans fullbemannade silvergruvor ger efter kalibreringen **20 silver/tick på nivå 1**, **36,470588 på nivå 3**. 20 nya, olika 64×64-kartor ger medel **27,875 / 50,830882**; de kan bära motsvarande **27,875 / 50,830882 spjutkohorter i bruttosold** totalt, inte per Wanax. Gruvplatserna är bara 20 på varje mätt karta. Inga ankartal räknas om.

Provenance: READ ONLY dev `http://10.0.1.92:8080/healthz` → `{"commit":"acf5c96","migration":176,"status":"ok"}`; DB `version=176, dirty=false`, värld `f7c90afa-4653-46a6-b14c-473a268efa3e`, 64×64, seed `1788982388878976733`, tick 150. Rådata: [dev-snapshot.jsonl](dev-snapshot.jsonl), [dev-healthz.json](dev-healthz.json). Nya kartor genererade av samma oförändrade Go-mapgen på `acf5c96f`; [mapgen.jsonl](mapgen.jsonl) och [mapgen.log](mapgen.log). DB-snapshotens faktiska filtid: 2026-10-12 22:58:40. Detta är en analytisk kapacitetsberäkning, inte ett genomspelat ekonomiscenario.

## Slice-kontrakt

| Fält | Kontrakt |
|---|---|
| Problem | Historiskt silverpris styr genesis och gudars värdering utan koppling till arbete. |
| Spelarsanning | Silverkostnad kan jämföras med gruvarbetets output; ett offer mäts i arbete. |
| Invariant | Timothy: kulle 1, kalkberg 2,5/arbetare/tick på nivå 1; befintlig +0,7/nivå; spjutkohort 1 silver/tick; startendowment inland730, kust1278 (Claudes besked23:06); koloni bär 30 ticks sold. Ingen ny tolkning av gamla events. |
| Scope | STOPP 1: full kod-/SQL-inventering och reproducerbar beräkning i denna rapportkatalog, dev bara READ ONLY. |
| Non-scope | Ingen balansändring i produktion, geografiförändring, audit-reparation, testvärld eller deploy. |
| Acceptans | Alla 16 katalograder redovisade; alla värdeläsare/silverflöden klassade; produktion och kedjor härledda; dev + ny mapgen + arméförsörjning; ren dokumentgren. |
| Stopvillkor | Hash + STOPP 1 till Claude före implementation. Förslag nedan är inte beslut. |
| Bevisplan | DB-snapshot i READ ONLY-TX, process-healthz, verklig mapgen, källhänvisningar, beräkningsassertioner och känslighetsprov av analysen. |

Kedjegrind: **blockerar** alfatestets ekonomi och begripligheten i geografi → brist → brons → elit. Första spelarytor: startsilver, gruvans nästa arbetare, offerpanel, varulista och arméns sold.

## 1. Alla värdeläsare

Sökning i hela repot efter `base_value`, `GoodBaseValue`, `baseValues`, `GenesisSilverLiquid` samt camel-case-varianter. Fulla träfflistor: [value-readers.txt](value-readers.txt), [value-runtime.txt](value-runtime.txt), [value-tests.txt](value-tests.txt), [migration-values-functions.txt](migration-values-functions.txt). Kommentars-/skrivträffar är separerade från faktisk läsning nedan. Ingen NPC-varumarknad hittades.

| Läsare/ägare | Källa | Nuvarande innebörd och föreslagen hantering |
|---|---|---|
| `GoodBaseValue` | `server/internal/economy/recompute.go:480` | SQL `SELECT base_value FROM goods`. Endast två produktionsanrop, båda grain, nedan. Frikopplas från genesis. |
| Metropolens seed **och cap** | `server/api/handlers/create_metropolis.go:153` | Grainvärde → `GenesisSilverLiquid(pop,base,cfg)`. Kran; tas ur varuvärdeskedjan i samma implementation. |
| Kolonins cap, **ingen seed** | `server/internal/combat/unit_arrival.go:2002` | Samma grainläsare → endast cap; måste också frikopplas. Kolonins amount är befintlig `carriedSilver`. |
| `GenesisSilverLiquid` / `genesisNeed` / `dailyGrainNeedInSilver` | `server/internal/economy/genesis_silver.go:21,30,40` | `min(pop,30000) × 0,005 × grain.base_value × cfg`, default seed×5, cap×10 (`sitos_config.go:85`). Är ingen dold marknad, bara en gammal genesisformel. |
| Gudarnas index | `server/internal/religion/index.go:64,72,119` | SQL-värde → rarity → smoothing → persistent `divine_valuations`. Byts gemensamt till arbetsvärde, gammal smoothing får inte blanda gamla prisvärden med dagsverken. |
| Legacy varumodell | `server/internal/economy/goods.go:49` | `Good.BaseValue` är en strukturfältsdeklaration; ingen faktisk produktionsläsare av fältet hittad. Tas med i namninventeringen. |
| Rent gudavärde | `server/internal/religion/valuation.go:126,134` | `base × (1 + rarityGain×rarity)`, smoothing. Math kan bevaras; indata/namn/enhet ändras. |
| Bönens offer, traditionella baseline och chans | `server/internal/religion/offering.go:50,60,90,153`; `server/api/handlers/settlement.go:1146,1159` | Indirekta konsumenter av indexet. Silvergolvet 100 gäller bara `OfferingWorth`, inte varuhandel; tas bort i föreslagen implementation. TraditionalBaseline och faktiskt offer måste använda samma nya mått. |
| Stående kult, faktisk maintenance | `server/internal/kharis/tick.go:1879,1903` | Hårdkodad `baseValues={oil:4,wine:5}`. **Stale:** live DB är 86,4/72. `divine/base` går redan över clamp 2 även utan knapphet. Måste dela ny arbetsvärdeskälla med index. |
| Stående kult, read-only projektion | `server/internal/kharis/project.go:96` | Samma hårdkodade `baseValues`, samma fel; måste ändras ihop med tick. Substitutionsmatningen använder också divine-index (`project.go:103`, `tick.go:1908`). |
| Matöversikten | `server/api/handlers/province.go:902` | Läser `g.base_value` till `grainBaseValue` men **använder inte talet efter Scan**. SQL/variabel kan rensas i implementationen. Ingen offerrekommendation här; det var en preliminär misstanke, avfärdad vid konsumentläsning. |
| Stadens varulista | `server/api/handlers/province.go:2914,2986` | Läser och serialiserar `base_value`; mängd/takt/cap/capacity/marginal_yield har andra källor. Förslag nytt explicit `labor_value`. |
| Keryx goods | `server/cmd/keryx/cmd_goods.go:68` | Läser JSON `base_value` och visar Base Value. Förslag Workdays med förklarad referens, aldrig Price. |
| Webb / allmän varukatalog | `web/static/js/megaron`; `server/api/handlers/goods.go:37` | Ingen direkt `base_value`-konsument i webb-JS. `/api/v1/goods` läser key/name/tier/category, inte värde. Kultpanelen får gudavärde/baseline indirekt från servern. |
| Testläsare | `server/internal/economy/metal_prices_test.go:53,86,117`; `sitos_test.go:47` | SQL-prisankare, brons 1,5× insatspris, silver-numéraire=1 och gammal popinvarians. Omskrivs till arbetskedja och startbeslut efter START; gamla assertioner är inte nya balansregler. `colonist_purse_test.go` nämner genesis historiskt. |

### SQL och migrationer

**Alla nuvarande public-funktioner är också lästa ur dev-DB:** `current_world_tick()` och `settled(amount,rate,calc_tick)`. **Ingen läser base_value.** `settled()` är en unclamped lat tuple och skapar inte varupriser. De kompletta definitionerna finns sist i snapshoten. Migrationernas `CREATE FUNCTION`/ersättningar är sökta; inga ytterligare funktionsläsare av base_value hittades.

* `008_economy_substrate.up.sql:13,17`: schema/default och första katalogen, skrivare. `018:7`, `019:4`, `025:12`, `031:14`, `052:11`, `055:8`: katalog-INSERT, skrivare. `118_remove_luxury.down.sql:4`: historisk down-INSERT av luxury, ingen aktuell vara.
* `072_sitos_fund.up.sql:22`: **faktisk historisk SQL-läsare**, seed `population × 0,5 × grain.base_value × 10` till silverfonden. Fonden/kolumnen borttagen i `106_sitos_granary.up.sql:59`; ingen livekonsument. `106:51` flyttade det redan bundna silvret till amount och cap, inte en återkommande kran.
* `136_dagsverkesskalan.up.sql:167–179`, down:14–26: **read-modify-write** av varje base_value med gamla produktionsdivisorer; en historisk reskalning, ändras inte.
* `139_metallpriser.up.sql:34`: tenn = aktuell koppar ×2; `:58`: brons = SUM(recept.quantity × insats.base_value) ×1,5. Down:9 läser koppar, :17 sätter brons20. **Historiska SQL-läsare**, inga liveprisfunktioner. Ny migration skriver arbetsvärde, inte dessa gamla migrationssemantiker.
* `093_divine_valuation.up.sql:28`, `095_colony_cap_retrofit.up.sql:18`, `107_colonist_purse.up.sql:4`: värde-/genesisomnämnanden i kommentarer, **inga läsningar**. 095 uppdaterar bara icke-silver-cap.

## 2. Alla silverkranar, sänkor och flyttar

Full inventeringsråvara med fil:rad: [silver-references.txt](silver-references.txt). Även generiska good/manifest-vägar har följts; de syns inte alltid i en sökning på ordet silver.

| Typ | Mekanik/källa | Vad den faktiskt gör |
|---|---|---|
| Kran | `api/handlers/nomadic_host.go:179,196` | Initialt 2 spearmen×1×240 = 480 i founder_phase. Tuple-rate −2 betalar sold under vandringen; första endowmentet. |
| Kran (återresning) | `api/handlers/rise.go:22–23,113`; `nomadic_host.go:179` | **Rise** efter dispossession anropar samma seedHost med1kohort×60ticks=**60** nytt silver. Ny startendowment även för återresare, inte bara första join. Nykomlingens730 skall inte oavsiktligt bli Rise730. Återgrundning får dessutom idag metropolens popseed genom samma createMetropolis. |
| Kran | `api/handlers/create_metropolis.go:153–156` | Vid 1000 invånare: 0,005×1000×129,6×5 = **3240**, cap6480. Tillsammans med ospenderat host480 blir det idag **3720**, inte3240. |
| Flytt | `api/handlers/found_metropolis.go:282` | Häller bara hostens återstående silver ovanpå city-seed; upplöser hosten. Förslaget nedan undviker dubbla kranar. |
| Kran | `economy/placement_yield.go:273`, `hex_rules.go:82,104,127`, `recompute.go` | Gruvarbetare på fyndighet, mine på exakt hex. Rate i settlement_goods → lat utvinning. Utan mine ingen silverregel. Nivåtrappan/platserna nedan. |
| Sänka + cirkulation | `combat/upkeep.go:79,528–537` | Bruttosold 1/2/3 eller 1,5/2,5/1 per enhet. Default soldShare0,7 krediteras tillbaka om garrison town = betalare. Brutto-afford gate kvar. I fält lämnar hela solden världen. |
| Sänka | `province/training.go:108–113`; `api/handlers/province.go:2019,2188`; `kharis/tick.go:1160` | Rekryterings- och förstärkningskostnad, silver bort från goods. Land ×100, skepp ×CrewFor, inte ×1. Ingen soldåtercirkulation av rekryteringskostnad. |
| Möjlig kostnadsväg, idag 0 silver | `province/building.go:8,77–93,214`; `api/handlers/province.go:1412` | Build kan debitera `CostSilver`, men samtliga aktuella specs/nivårecept saknar det: byggnader kostar material. Ingen aktuell byggsilversänka. Skeppsreparation `combat/ship_repair.go:61,80` kostar bara timber/cedar, inget silver. |
| Sänka | `api/handlers/settlement.go:1135–1300`; `religion/prayers.go:263–267` | Offer debiterar exakt varurecept, silver om kompositionen innehåller det; även tyst svar kostar offer. Gamla böner har historiska recept; fem nya aktiva listade nedan. |
| Sänka, ibland via substitution | `kharis/tick.go:1908,1933`; `kharis/project.go:103,158` | Stående kults traditionsoffer oil/wine; substitution exkluderar **silver** i SQL. Ingen dold återkommande silverkostnad för tempel. |
| Sänka | `economy/tithe.go:23–42`; `economy/trade.go:245–273` | Religiös varuförsäljning med tempel och silverbetalning≥10: **10% försvinner**; toSeller krediteras, inget faktiskt silverkonto för templet. TempleTithe-event, inte ny kran. |
| Sänka | `ai/governor.go:49–57`; kallad `kharis/tick.go:343` | AI-guvernör, loyalty≤2 och stock≥20: debiterar10. Kommentaren lovar gift-event/stabilisering men kroppen gör bara avdraget. Dokumenterat fynd, ingen reparation i denna slice. |
| Flytt / escrow | `api/handlers/messenger.go:1342,1429`; `economy/trade.go:89,110,286`; `trade_return.go:74` | BUY escrow, SELL/bilateral avtal, acceptans, silvertransport, varuretur och expiry-refund flyttar redan befintliga mängder. Spelarna sätter priset; base_value ingår inte. |
| Flytt | `api/handlers/settlement.go:635`; `province.go:3439`; `logistics.go`; `economy/gift.go:115–127`; `transport/arrival.go:150` | Egna transfers, gåva/tribut och standing orders debiterar avsändare och flyttar samma manifest. Ingen skillnad i silverrisk pga ägare. |
| Flytt | `combat/march_start.go:873–891,1259`; `combat/unit_arrival.go:1992–2005,500,526`; `api/handlers/unit.go:1477` | Kolonipung debit→carried_silver→koloni; retur/disband kan flytta tillbaka pungen. **Kolonier seedar inget nytt silver.** 30 ticks sold utifrån kolonisttyp/storlek. |
| Flytt / förlust | `combat/occupation.go:657,668,747`; `transport/intercept.go`; `combat/unit_intercept_scan.go` | Plundring/överfall flyttar gods genom fysisk last; silver är good med egen lootvikt. Ingen mottagande metropolis kan förlora loot (`occupation.go:761`). Ägarbyte av stad byter tillgångens ägare, ingen ny kran. |
| Sänka / otillgänglig stock | `transport` last/förlisning, `combat` enhetsförlust/kollaps, carrier-vittnen | Förlorad fysisk last/pung eller övergiven/raserad stad kan ta silver ur användbar cirkulation. Inte ett slumpmässigt handelspris eller ny valuta. |
| Sänka vid overflow | `transport/arrival.go:156`, `economy/trade.go:289`, `gift.go:127`, `unit_arrival.go:500`, `unit.go:1480` | Mottagningsvägar klipper mot cap eller rapporterar CreditedQuantity/spill; lagertak är viktigt för faktisk leverans. Lat mined rate är inte samma sak som nettot som förblir tillgängligt. |
| **Ingen** silvermekanik | `economy/sitos_tick.go:49`; migration106 | Sitos är food-only. Ingen avgift, NPC-kornköpare eller ny silverfond. |
| Mätare | `combat/silver_audit.go:32,48,76,103` | Summerar active liquid + pending BUY escrow; fund_total0. `mined_since_last = aktuell rate × elapsed ticks`, inte realiserad/minneshistorisk nettoutvinning. **Missar founder_phase, unit.carried_silver och transportmanifest**; därför inte ett fullständigt bevarandeprov. Eventfältens äldre innebörd bevaras. |
| Gated/legacy | `api/handlers/kingdom.go`, `capabilities/kingdom_verbs.go`, `province/model.go` | Kingdomsilverytor post-MVP/gated. Legacy ResourceLedger är readmodell från goods, ingen självständig silverkolumn på settlement och ingen kran. |

Migrationssilver är engångshistorik: 025/041/057 ändrade valutaägare/namn/lager; 061/154/155 gruvmodell; 072 fondseed; 096 korrupt-enhetsklamp; 106 konservativ fondflytt; 107 pung; 133 provisionsomläggning; 136 skala; 139 metallvärde. De är ingen daglig produktion. En färsk DB måste fortfarande kunna köra hela kedjan oförändrat fram till en ny migration.

## 3. EN gemensam tabell: samtliga varor och enheternas resursåtgång

**Definition av dagsverke:** en gubbes arbete under en tick. Ett arbetsvärde är `1 / standardtakt`; brons adderar insatsvarornas arbetsbörda och gjutaren. Referensytor är uttryckligt valda nedan, utan produktivitetshöjande byggnad. **Silver är ett beslutat undantag med obligatorisk mine nivå1.** Byggnadsinvesteringar, lokal knapphet, transporter och högre nivåer ingår inte i basvärdet; gudarnas knapphetsmultiplikator läggs separat ovanpå. En billigare/bättre lokal yta ändrar faktisk arbetstakt, inte katalogens referensvärde.

Tabellens tal kommer från **live-tabellen** ([dev-snapshot.jsonl](dev-snapshot.jsonl), `production_rules`), inte historiska råtal i balansvaulten. `S:n` nedan betyder rad n i denna snapshot. Produktionskodens källa är `server/internal/economy/hex_rules.go:104,127`, `placement_yield.go:317` och `recompute.go:690,807`. Base-value-källorna för samtliga varor är snapshotrader3–18, respektive migrations136/139 ovan. Kolumnen ny är ett förslag i **dagsverken**, aldrig ett nytt silverpris.

| Vara / enhet | Standardyta och en arbetares output/tick | Dagsverken/enhet, härlett | Dagens base_value | Föreslaget arbetsvärde | Resursåtgång per enhet; för sold: grain+silver/tick och rekrytering | Källa fil:rad |
|---|---|---:|---:|---:|---|---|
| grain | plains, 1 | 1/1 = **1** | 129,6 | **1** | Ingen insatsvara | S:19; hex_rules.go:54 |
| fish | coastal_sea, 1 | **1** | 172,8 | **1** | Ingen; river0,5→2 DV, deep_sea1/6→6 DV är alternativytor | S:41–43,51; hex_rules.go:71 |
| livestock | plains, 1 | **1** | 180 | **1** | Ingen insatsvara i produktionsregeln | S:28; hex_rules.go:54 |
| timber | forest_olive_grove, 1 | **1** | 864 | **1** | Ingen; lumbermill är frivillig effektivisering | S:35; hex_rules.go:63 |
| cedar | forest_cedar, 1 | **1** | 576 | **1** | Ingen; 3 basplatser per hex ändrar inte en arbetares takt | S:44; hex_rules.go:65 |
| copper | hills+deposit, 1 | **1** | 172,8 | **1** | Ingen; mine är frivillig effektivisering av copper | S:23; hex_rules.go:75 |
| tin | mountain_limestone+deposit, 1 | **1** | 345,6 | **1** | Ingen; inga extra knapphets-DV: knapphet läggs i divine-index | S:24; hex_rules.go:78 |
| stone | ordinär landyta/plains, 0,5 | **2** | 14,4 | **2** | Ingen; hills1→1 DV, limestone2→0,5 DV. Fältsten vald som vanlig referens. | S:55–62,26–27; hex_rules.go:48 |
| silver | hills+deposit+mine L1, **1 beslutad** | **1** | 1 | **1** | Ingen insatsvara; limestone L1 ger2,5→0,4 DV lokalt | S:32–33 (live r0); hex_rules.go:82,104; Timothys ankare i order |
| oil | grove, rå1; L1-presskedja max8/3 per extractor | Rå **1**; P6 full kedja **9/16=0,5625** | 86,4 | **1**, standard utan effektivisering | Inga lagrade olives/receptinsatser. P6: 1 extractor +0,5 refiner →8/3 oil, bådas arbete inräknat. Vid heltalsbemanning 1+1→8/3 ger0,75 DV. | S:39–40,52; recompute.go:690–726 |
| wine | hills, rå1; L1-winerykedja max3,5 per extractor | Rå **1**; P6 full kedja **3/7≈0,428571** | 72 | **1**, standard utan effektivisering | Inga lagrade grapes/receptinsatser. P6: 1 extractor +0,5 refiner →3,5 wine. Heltal1+1→3,5 ger4/7 DV. | S:21,31,53; recompute.go:690–726 |
| bronze | foundry L1, 1; inputarbete tillkommer | 9×1+1×1+1 = **11** | 2851,2 | **11** | **9 copper +1 tin**, 1 gjutartick; OutputQty1. Inget påhittat timber/fuel-recept. | S:54,63–65; recompute.go:841; migration099; recipe.go:4 |
| horses (**parked**) | dormant stable L1-row, 1 | **1** rent hypotetiskt | 432 | **1 som dormant referens**, ej tillgänglig | Ingen aktiv produktionsväg eller foderingrediens i DB; lova inte att horses kan produceras | S:25; recompute.go:106; placement_yield.go:395 active-filter |
| pottery (**parked**) | dormant market L1-row, 1 | **1** rent hypotetiskt | 108 | **1 som dormant referens**, ej tillgänglig | Ingen clay/inputrad i DB; ingen aktiv produktion | S:37; recompute.go:105; placement_yield.go:395 |
| purple (**parked**) | **Ingen production_rules-rad** | **Ej härledbart** | 388,8 | **NULL/ej tillämpligt**, uteslut ur offer | Ingen aktiv snäck-/färgkedja att räkna; ingen gissad dagsverkesiffra | S:13; hela production_rules S:19–62; mig115:16 parked |
| cult (**inte vara**) | temple devotion, separat labor | **Ej tillämpligt** | 0 | **NULL/ej tillämpligt**, ej offer/handel | Arbete/relationship, ingen flyttbar katalogvara | S:6; mig094; kharis/tick.go:557 |
| spearman, 100 man | Ingen varuproduktion | — | — | — | **Garrison0,50 grain+1 silver/tick; fält1 grain+1 silver**. Rekryt **12 grain+20 silver** | combat/upkeep.go:79,98; province/training.go:108 |
| elite_infantry, 100 man | — | — | — | — | **0,60 grain+2 silver/tick; fält1,20+2**. Rekryt **24 grain+40 silver+16 bronze** (176 DV bronskedja) | upkeep.go:80; training.go:111 |
| war_chariot, 100 man | — | — | — | — | **0,80 grain+3 silver/tick; fält1,60+3**. Rekryt **40 grain+50 silver+8 timber+8 cedar+20 bronze** | upkeep.go:81; training.go:109 |
| galley, 1 skrov/20 crew | — | — | — | — | **0,04 grain+1,5 silver/tick**, ingen fältdubbling. Rekryt **0 grain+6 silver+30 timber** | upkeep.go:82; training.go:110; unit/model.go:64 |
| war_galley, 1 skrov/50 crew | — | — | — | — | **0,06 grain+2,5 silver/tick**. Rekryt **0 grain+30 silver+100 cedar** | upkeep.go:83; training.go:112; unit/model.go:66 |
| merchantman, 1 skrov/10 crew | — | — | — | — | **0,03 grain+1 silver/tick**. Rekryt **0 grain+2 silver+16 timber** | upkeep.go:84; training.go:113; unit/model.go:68 |

**Oil/wine-kedjan:** `output = baseline×extractors + min(boost×extractors, refiningRate×refiners)`. Optimerad kontinuerlig bemanning vid L1 balanserar `refiners/extractors=boost/refiningRate`. Allt arbete per output är alltså `(1+boost/refiningRate)/(baseline+boost)`. Att bara räkna `1/refiningRate` missar råvaruarbetet; att lägga på ett fiktivt olives/grapes-recept ändrar spelet. Eftersom råoljan/råvinet redan är samma katalogvara föreslås referens1, med den verkliga P6-effektiviseringen redovisad ovan. Ingen separat förädlad vara finns.

**Bronseffektivisering:** två mine L1 kan ge copper/tin×1,7: faktisk kedja9/1,7+1/1,7+1 =6,882353 DV. Det ändrar inte referenskedjan11, precis som en farm inte ändrar grain-basvärdet. Bronsens gjutarkapacitet nivå1 är1/tick och ingen högre foundry är spelbart byggbar (`province/building.go:127` saknar foundry i LevelledBuildings).

**Enhetstal:** landkostnader ovan är UnitSpecs per-man×100; naval per-man×CrewFor. `UnitCatalogue` gör exakt denna multiplikation (`api/handlers/province.go:1906–1914`), liksom rekryteringshandlingen. Inga gamla PopCost-tal används för antal soldater. Reinforce debiterar samma per-man-kostnad för faktisk påfyllning.

## 4. Maximal silveroutput, inte bara en arbetare

**Platser och nivåformel:** silver har P0=1; mine ger +4 ⇒ **5 silverplatser per hex på ALLA byggbara nivåer**, inte 5×nivån (`hex_rules.go:27,82,104`). Vid full bemanning 5 arbetare/hex. `MaxBuildingLevel=3` (`province/building.go:112`). Multiplikator idag `1+0,7L`; ingen ny nivåmodell införs här. För exakt beslutade L1-output1 respektive2,5 blir nya r0:

```
hills r0 = 1/1,7 = 0,588235294118
limestone r0 = 2,5/1,7 = 1,470588235294
r(L) = ankare(L1) × (1+0,7L)/1,7
L1: hills1; limestone2,5
L2: hills24/17=1,411765; limestone60/17=3,529412
L3: hills31/17=1,823529; limestone155/34=4,558824
hex-output: 5 × respektive arbetartakt
world-output: 5 × (antal_hills + 2,5×antal_limestone) × nivåfaktor
```

`anchor×(1+0,7×(L−1))` vore en annan trappa; används **inte**. Ordnings-/vaultens avrundade dagens2,3/5,8 är live exakt **2,304/5,76**, dvs r0=1,355294117647/3,388235294118 gånger1,7. Dev-taket före kalibrering är4×5×2,304=**46,08** på L1 (L3=84,028235). Ankarna1/2,5 har inte ersatts med en bokstavlig division av de avrundade talen.

**Dev READ ONLY:** alla fyndigheter, inga utelämnade pga nuvarande ägare eller catchment: **4 hills, 0 limestone**. Snapshotens `silver_tiles` är en GROUP BY över alla map_tiles med silver_deposit. 4×5 platser×1 =20 silver/tick på L1; L3=4×5×31/17=36,470588. Dagens verkliga miners behöver inte vara fullt bemannade; detta är en fysisk övre potential. Antagandet kräver mine och fem placerade gubbar på varje fyndighet, inga blockader/belägringar eller competing occupancy, nåbara bosättningar, mat och lagerutrymme.

**Ny 64×64:** mapgen beräknar `players=max(10,round(4096/529))=10`, silver target=max(4,players/3)=4, en hex/source (`world/mapgen.go:437–449,480,497,1691`). Det är **inte** world.max_provinces100. 20 spridda begärda seeds `1009×1..20` gav 20 **olika effektiva seeds**, fyra silverhex vardera. De extra seeds0–19 finns med men används inte i medel: reseeding återanvände samma effektiva karta där. Dev-seeden regenererades också och gav samma fyra hills.

| Karta | Silverhex hills / limestone | Silverplatser = fullt bemannade arbetare | L1 silver/tick totalt / per10 Wanax | L3 silver/tick totalt / per10 Wanax |
|---|---:|---:|---:|---:|
| Dev, uppmätt | 4 / 0 | 4×5=20 | **20 / 2** | **36,470588 / 3,647059** |
| Nya, observerat lågpunkt (flera seeds) | 4 / 0 | 20 | 20 / 2 | 36,470588 / 3,647059 |
| Nya, representativ median (seed1009→1011) | 3 / 1 | 20 | **27,5 / 2,75** | **50,147059 / 5,014706** |
| Nya, medel av20 olika kartor | 2,95 / 1,05 i snitt | 20 | **27,875 / 2,7875** | **50,830882 / 5,083088** |
| Nya, observerad högpunkt (3027→3029) | 1 / 3 | 20 | **42,5 / 4,25** | **77,5 / 7,75** |
| Teoretiskt tak om alla4 vore limestone, ej observerat | 0 / 4 | 20 | **50 / 5** | **91,176471 / 9,117647** |

**Per Wanax är normalisering, inte rättighet till en gruva.** Fyra separata källor gör fördelningen ojämn. Dev har1 registrerad och1 aktiv stadsägare, så `/faktiska dev-Wanax` = hela dev-outputen. Tabellen använder mapgens10 som jämförelsepopulation. För50 alfatestare: dela total med50, t.ex. medelkartan L1 **0,5575**, L3 **1,016618 silver/tick/Wanax**. För world-konfigurationens100:0,27875/0,508309. Med samma20 gruvarbetare på kartan finns inte en egen gruva för alla dessa spelare. Därmed finns ett geografiskt handels-/tributberoende; detta är en observation, inget nytt geografibeslut.

## 5. Arméer som maxoutputen försörjer

Beräkning: `output / UnitUpkeep.Silver`. Tabellen visar **kontinuerliga enhetsekvivalenter totalt / per10 Wanax**; enheter kan inte rekryteras som bråk. För en verklig hel värld avrundas totalt nedåt med `floor`; de fullständiga heltalstaken finns i [calculations.json](calculations.json). Varje kolumn är ett alternativ där **all** output går till just den typen, aldrig sex arméer samtidigt. Det är bruttosold, inte en simulering av mat, rekrytering, bemanning eller krigsförluster.

| Outputscenario/nivå | Spjutkohorter, sold1 | Elitkohorter, sold2 | Stridsvagnskohorter, sold3 | Galärer, sold1,5 | Krigsgalärer, sold2,5 | Handelsskepp, sold1 |
|---|---:|---:|---:|---:|---:|---:|
| Dev L1 | 20 / 2 | 10 / 1 | 6.66667 / 0.666667 | 13.3333 / 1.33333 | 8 / 0.8 | 20 / 2 |
| Dev L3 | 36.4706 / 3.64706 | 18.2353 / 1.82353 | 12.1569 / 1.21569 | 24.3137 / 2.43137 | 14.5882 / 1.45882 | 36.4706 / 3.64706 |
| Nya medel L1 | 27.875 / 2.7875 | 13.9375 / 1.39375 | 9.29167 / 0.929167 | 18.5833 / 1.85833 | 11.15 / 1.115 | 27.875 / 2.7875 |
| Nya medel L3 | 50.8309 / 5.08309 | 25.4154 / 2.54154 | 16.9436 / 1.69436 | 33.8873 / 3.38873 | 20.3324 / 2.03324 | 50.8309 / 5.08309 |
| Nya observerat max L1 | 42.5 / 4.25 | 21.25 / 2.125 | 14.1667 / 1.41667 | 28.3333 / 2.83333 | 17 / 1.7 | 42.5 / 4.25 |
| Nya observerat max L3 | 77.5 / 7.75 | 38.75 / 3.875 | 25.8333 / 2.58333 | 51.6667 / 5.16667 | 31 / 3.1 | 77.5 / 7.75 |
| Teoretiskt alla4 limestone L1 | 50 / 5 | 25 / 2.5 | 16.6667 / 1.66667 | 33.3333 / 3.33333 | 20 / 2 | 50 / 5 |
| Teoretiskt alla4 limestone L3 | 91.1765 / 9.11765 | 45.5882 / 4.55882 | 30.3922 / 3.03922 | 60.7843 / 6.07843 | 36.4706 / 3.64706 | 91.1765 / 9.11765 |

Exempel på heltal: dev L1 **20 spjut /10 elit /6 stridsvagn /13 galärer /8 krigsgalärer /20 merchantmen**. Dev L3 **36/18/12/24/14/36**. Nya medelkartans matematiska tak L1 **27/13/9/18/11/27**; detta är floor av medelpotential, inte en viss fysisk karta. Representativ median27,5 L1 ger samma heltal.

**Cirkulationen ändrar nettoförsörjningen:** nuvarande default `UPKEEP_SOLD_SHARE=0,7`, om enheterna garnisonerar hos sin betalare, gör nettosänkan0,3×bruttosold. Då kan samma mine-output i steady state täcka **3⅓ gånger** tabellens bruttosoldsekvivalenter, med tillräcklig stock för hela afford-gaten i varje debitering. Dev L1 motsvarar nettopotential66,6667 spjutkohorter; fältarméer får inte denna återbäring. Ankaret ”1 gruvarbetare försörjer1 kohort” är exakt som **bruttosoldreferens**, inte garrisonens nettodränering. Ingen cirkulationsregel har ändrats eller live-env antagits; jämförelsen är dokumenterad default.

**Startarméns gruvarbetare:** 2 fulla spjutkohorter kräver2 silver/tick → **2 hills-L1-arbetare**, eller0,8 limestone-arbetare matematiskt (**1 faktisk gubbe**). L3:2/(31/17)=1,096774 hillsarbetare →**2 gubbar**, limestone2/(155/34)=0,438710→**1 gubbe**. Med default garrisonåterbäring behöver armén0,6 nytt silver/tick →**1 hills- eller limestone-gubbe** redan påL1, men måste ha2 silver tillgängligt vid betalning.

**Kuststart är annorlunda:** Poseidons startgalär stöds av staden (`api/handlers/found_metropolis.go:326`), kostar ytterligare1,5 silver/tick och0,04grain; total2kohorter+galär =**3,5 silver/tick**. Brutto kräver**4 hills-L1-gubbar** eller**2 limestone-L1-gubbar**. Garrison-netto vid default är1,05/tick →2 hills-gubbar eller1 limestone-gubbe. **Nytt besked från Claude23:06 (Timothy): ankaret365×VÄRDENS faktiska sold omfattar stödgalären. Inlandskassa730; kustkassa365×3,5=1277,5, beslutad avrundning1278.**1278/3,5=365,142857 ticks bruttosold (ingen mine/återbäring). Detta ersätter ursprungsorderns generella730 för kuststart; soldtabellen är oförändrad.

Gruvarbetarna behöver också mat. En gubbe=100 invånare har0,5 grain/tick i civil behovsreferens;20 gruvarbetare motsvarar2000 invånare och10grain/tick innan övriga invånare/armé. Det är inte medräknat som en ny silverkostnad. Minebygge/nivåer kräver material och tid. ”Kan bära i sold” garanterar inte att en ny spelare kan mobilisera dessa arméer från sin startpopulation.

## 6. Konkreta förslag för Claudes beslut

1. **Gemensamt arbetsvärde:** lägg ett explicit `goods.labor_value` (nullable för cult/purple) med tabellens standardvärden. Låt gamla `base_value` vara historiskt/deprecated tills kompatibilitetsbehovet är löst; inga nya produktionsläsare använder det. Detta undviker att ett fält fortsatt ser ut som silverpris. De12 aktiva riktiga varorna får en definierad arbetsreferens; parked horses/pottery får endast dormant referens och skall fortfarande inte bli offerbara/producerbara. Cult/purple utesluts ur nya värdeindexet. Kharis tick **och** ProjectDailyNet hämtar samma värden som religion-indexet, ingen ny hårdkodad parallell karta.
2. **Genesis i samma implementation:** startendowment **inland730, kust1278 totalt inklusive det hosten bär**, enligt Claudes nya besked23:06. Inte ett nytt fullbelopp ovanpå hostens saldo. Förslag: newcomer-hostens inlandgrundendowment365×2=730; kustgrundning kompletterar till beslutad kustendowment1278, med hostens verkliga saldo inräknat och utan pop/varuprisformel. Separat initial endowment och flytt måste hållas isär. En äldre host kan redan ha spenderat sold på vägen: implementationen behöver låsa om365-ticksankaret avser ursprunglig endowment eller nytt faktiskt saldo vid grundningsögonblicket; ingen tyst återbetalning av tidigare sold föreslås här. Rise-specens1kohort×60ticks=60 skall lämnas explicit oförändrat; dela inte newcomerens365-ticksflagga med alla seedHost-callers. Omedelbar grundning ger730 inland/1278 kust. Återgrundning/Rise får bara sitt kvarvarande60-endowment; **Claude23:06 godkänner att den aldrig får extra metropolkran**. Frikoppla city-cap från grain och population. Förslag metropolis cap**1460 inland /2556 kust** (nuvarande2×-relation bevarad, numera2×startendowment). Hantera både normal hostgrundning och direkta createMetropolis-anrop så en andra startkran inte kan införas av en intern caller.
3. **Koloni oförändrad pungregel:** full spjutkolonist debiterar**30 silver**, elit60, stridsvagn90 genom befintlig UnitUpkeep×30; aldrig genesisseed. Föreslagen cap30×sold×2, minst **60** för den vanliga spjutkolonin, vid grundning `max(referenscap,verklig_pung)` så redan buren valuta inte förstörs. Kolonipung är inte nya pengar. Äldre stadsstock/backfill behöver bevaras: inte skriv cap under befintligt settled stock utan separat beslut om överskottet; inga stock-omskalningar med dagsverkeskvot.
4. **Bönernas silver:** förslag Vision10grain+**3silver**, Green15grain+**2silver**, Thread4timber+**3silver**, Passage6fish+**3silver**; Walls8grain+4timber oförändrad. Bara silverbeloppen sänks (förslag), väntetid/chans/kharis-gates/effekter bevaras. Utan knapphet/favours blir referensarbetsbördan13/17/7/9/12 DV. Potnias affinity vägs separat. Ny TraditionalBaseline följer nya receptet, så det normala offerreceptet fortfarande är normal chansreferens. SilverAltarValueFloor tas bort, inte ersätts med ett nytt golv.
5. **Katalog och eventmått:** Keryx goods kan visa `Workdays/unit` med förklaring att detta är gudarnas referensarbete; webbens katalog kan visa samma hjälptext, eller lämna bort talet från handelsflödet. Spelarnas offertpris förblir fritt. Codex goods/rites/kharis förklarar måttet och silverankaret. Nytt payloadfält med explicit arbetsenhet för nya events, historiska `RiteCast.offering_worth` läses som sitt historiska mått. Gamla effekt-/eventpayloads omskrivs inte. Implementation skall avgöra om nya eventtypen behöver versioneras (regel: samma befintliga fält får aldrig börja betyda något annat).
6. **Indexbyte:** nya arbetsvärden får inte smoothe från gamla numerärer (oil86,4→1 osv). Föreslå separat ny indexenhet/version och initial recompute före nya måttläsare. Bevara scarcity-formel, holderfloor och befintliga dagliga underhållsrecept; identifiera kharis-flow-förändringen från den stale denominatorns reparation i tester, inte som en tyst följd av ett rent kolumnbyte.

De här förslagen är **inte implementerade**. Ankarna (1/2,5,0,7,1sold,365ticks startkassa, inland730/kust1278 enligt23:06,30kolonipung, Rise60) är beslut; standardyta för stone, dormant values, kataloglayout, prayer-silver2/3, cap1460/2556/60 och övergångsmekanismen är härledda/föreslagna och väntar på Claude.

**Väntande riktning (Claude23:06, ej beslut):** spjutkohorter kan få kornransoner i stället för silversold, silver bara till yrkeskrigare. Ingen ändring byggs eller räknas som redan giltig. Spjutkohortens kornåtgång är idag **0,5 dagsverke/tick i garnison,1 i fält** på grain-referens1 DV/enhet; rekryteringen12grain är**12 dagsverken** och årlig garnisonsration182,5 DV (fält365 DV). Tabellen för varje typ hålls separat så ett kommande soldbeslut kan ändra enbart rätt kolumner.

## 7. Bevis, grindar och kända avgränsningar

Reproducera aritmetiken: `python3 docs/reviews/silverkalibrering/calculate.py` → PASS. [calculations.json](calculations.json) innehåller alla tal inklusive sex armétyper och heltalstak. DB-skriptet [dev-readonly.sql](dev-readonly.sql) kör bara SELECT inuti `BEGIN READ ONLY` och slutar med COMMIT. Mapgenprovet [mapgen.go.txt](mapgen.go.txt) kör verklig GenerateMap, inte en handbyggd tilefixtur; [reproduce-mapgen.sh](reproduce-mapgen.sh) kopierar det till en tillfällig command-directory under serverns internal-importgräns, kör det och tar bort directoryn. Ingen produktionsfil kvarstår.

| Grind | STOPP 1 |
|---|---|
| Kod | Ingen produktionsdiff; analysassertioner, käll-/snapshotkontroll och mapgen körda. Full färsk Go/JS/vet och effektmutationer hör till STOPP2 efter START. |
| Visuell | Ingen spelaryta ändrad. BILD1280/390 av goods/kult skall göras efter implementation, inga gamla bilder hävdas som nya. |
| Semantisk | Enheten arbete är åtskild från spelarpris, fysisk output, bruttosold och garrisonnetto. Alla16 goods och6 rekryttyper finns i samma tabell. |
| Användare | Ingen native gameplay körd/ändrad i STOPP1; inga claims om nåbart tempel/gruva eller naturlig brons-/elitekonomi. |
| Drift | Endast read-only SELECT + healthz på dev; inga writes, env-läsningar, restart, push eller deploy. |

Känslighetsprov muterar **analysens indata**, inte produktion: grain-standardtakt1→2, bronsrecept9→8 och silverplatsantal4→3 skall ge namngivet assertion-rött; därefter byteåterställning och PASS. Det är prov av rapportens beräkningar, **inte** bevis av den ännu obefintliga produktionskalibreringen. Logg [analysis-verification.txt](analysis-verification.txt).

Avgränsningar: sample20 är en redovisad deterministic sample, ingen garanti för alla möjliga seeds; teoretisk output för4limestone har inte observerats. Dev är1 aktiv stadsägare, inte ett typiskt fördelat10-/50-spelarparti. Maxutvinning antar full bemanning/nåbarhet/mat; soldtabellen antar inga andra sänkor och alternativ homogent arméval. Parkerad purple har ingen kodhärledbar produktion. Balansvaulten har gamla priser/rater/kostnader och skall inte användas som livekatalog; uppdatering av dess relevanta mekanik hör till implementationen. Audit-/AI-fynden är rapporterade för triage, inte extra kodscope.

Metodisk lärdom: läs effektiva DB-tabeller efter hela migrationskedjan; håll brutto/netto åtskilda; deduplicera effektiva mapseeds efter reseeding.
