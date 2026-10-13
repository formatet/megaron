# Silverutredningen — STOPP

Utredning från master `97712ede`, enligt `.agents/order-silverutredning.md`, 2026-10-13. **Ingen produktionskod ändrad.** Timothy beslutar om geografi, spjutsold och startbuffert. Rapporten **väntar på** kedjegrinden geografi → brist → brons → elit; den visar kapacitet och geografisk fördelning, inte att en naturlig spelare har genomfört kedjan.

**Rekommendation:** pröva **fler fyndigheter, mål 10 på 64×64, fem platser per fyndighet**. Behåll takten 1 på kulle och 2,5 på kalkberg vid nivå 1. Det ger i detta urval 66,125 silver/tick vid nivå 1 och 120,581 vid nivå 3, mot 27,875/50,831 idag. Första tio värdstartarnas medianavstånd sjunker 15 → 5 hex, men bara 54 % hamnar inom åtta hex. Detta är därför en förbättring, **ingen rättvisegaranti**. Dubbla arbetsplatser på fyra fyndigheter löser inte avståndsproblemet. Gör inte både större antal och större gruvor samtidigt som B utan ytterligare behovsbelägg.

**B rekommenderas som kanonförslag:** vanliga spjut får behålla sina kornransoner och slippa löpande silversold; elit, stridsvagnar och skepp behåller sina tal. Det tar bort 100 % av nominell sold för dagens enda dev-värd och 57,14 % för en kuststart. Det är inte en uppmätt andel av en mogen världs alla silverutgifter. Föreslaget beslut att lägga framför Timothy: *behåller vi 730 inland/1278 kust som fasta startbuffertar, Rise 60 och kolonins 30 som separata buffertar när kopplingen till spjutsold försvinner?* Inga av dessa ändringar byggs här.

## Metod och reproduktion

Kontrakt skrevs före analysskripten: samma 20 begärda seeds `1009 × 1..20`, verklig `world.GenerateMap`, oförändrad takt och spacing. `run.py` kopierar Go-modulen och produktionspaketet `world` till en temporär katalog och ändrar **endast kopians** `silverSourceFloor` till 4/5/10/20/40. Inga repo-filer under server/web ändras. Vid 64×64 ger `playersFor` 10 tänkta spelare oavsett om 50 faktiskt försöker ansluta. Mål 5 motsvarar en fyndighet per två tänkta spelare; mål 10 en per tänkt spelare. Mål 20/40 är fasta stressnivåer, inte nytt befolkningsbeslut.

Starttestet extraherar den **exakta SQL-strängen** ur `api/handlers/join.go:274–415` och kör den i PostgreSQL 16 med temporära tabeller och verkliga karttiles/landmassor. För varje karta ansluts upp till 50 stillastående aktiva värdar i ordning, en gång, med PG `setseed(i/21)` för RANDOM-tiebreak. Alla statiskt möjliga starthex före första värden inventeras dessutom, inte bara markerade byar. Träförsörjningsfiltret är `forest_olive_grove`, verifierat mot mig177/dev. Detta är SQL-replay av placeringen, **inte 1 000 registrerade HTTP-spelare, grundningar eller flyttande värdar**. Första10 och upp till50 är samma prefix, inte separata godtyckliga samplingar.

Båda körningarna, med separata temporära PG-instanser, blev byte-identiska för kartor och samtliga valda startpunkter. Alla fem varianter behöll samma effektiva seeds, samma terräng och koppar/tenn/ceder som baseline på **20/20 kartor**. Därmed finns ingen omseedad geografikonfundering i just denna jämförelse. Nyckeln `non_silver_map_sha256` mäter dessa fält; silverflaggan undantas avsiktligt.

```sh
python3 docs/reviews/silverutredning/run.py
python3 docs/reviews/silverutredning/verify.py
python3 docs/reviews/silverutredning/payroll.py
# oberoende full omkörning:
SILVER_STUDY_OUT=/tmp/silver-study-repeat python3 docs/reviews/silverutredning/run.py
python3 docs/reviews/silverutredning/verify.py docs/reviews/silverutredning/results /tmp/silver-study-repeat
```

Go, Python3 och Docker med `postgres:16-alpine` krävs. Körningen använder inga dev-anslutningar; containern tas bort i `finally`. [manifest](results/manifest.json), [exakt SQL](results/spawn.sql), [aggregat](results/summary.json), [seeds](results/seeds.json), [verifikation](verification.log), [mutationer](mutation-check.log). Varje karta sparar `deposits=[q,r,terrain,landmass]` och **varje** kandidat/värd som `[q,r,närmaste_axialavstånd,närmaste_på_samma_landmassa_eller_null,landmassa]`, i `results/maps-{4,5,10,20,40}.jsonl`. Ingen startpunkt filtreras bort för att den ligger långt från silver.

Två första READ ONLY-frågor till dev använde fel kolumnnamn och avbröts utan skrivningar. Den kompletta korrigerade transaktionen finns som [SQL](dev-readonly.sql) och [snapshot](dev-snapshot.jsonl), mig177, tick150, en Wanax. [Healthz](dev-healthz.json) bekräftar `97712ed`/177. Råsnapshoten innehåller också psql:s BEGIN/COMMIT-rader; JSON-läsaren filtrerar enbart dessa kontrollrader.

## A — kapacitet

Vid L1: `platser × (kullehex × 1 + kalkbergshex × 2,5)`. Vid L3: samma × `(1+0,7×3)/(1+0,7×1) = 31/17`. Fem platser följer `economy/hex_rules.go:28,104`; arbetsplatserna ökar inte med nivån. Raterna följer mig177:14–17. L3 är nuvarande högsta gruvnivå (`province/building.go:112`). Varianter med tio platser är kontrafaktisk kapacitet på **samma kartor**, utan ändrad takt; de innebär silver-specifika extra platser. Att höja globala `BuildingExtraPlaces` skulle även höja andra varors kapacitet och är en annan ändring.

Varje siffra nedan är medel av 20 kartors fullbemannade övre potential. Gruva på varje fyndighet, bosättning som kan nå hexen, mat, arbetare och lagerutrymme krävs. En arbetare är en gubbe, inte en enskild soldat. Inte all denna produktion är omedelbart tillgänglig för en ny stad eller jämnt delad mellan spelarna. Bruttokohorter kostar 1/tick; garnisonskolumnen använder default netto 0,3/tick **hos den egna betalaren**. Den verkliga betalningsspärren kräver fortfarande full sold varje debitering. Tabellen visar kohortekvivalenter; hela förband behöver avrundas ned när en konkret armé dimensioneras.

| Väg | Hex faktiskt, snitt (min–max) | Platser/hex | Arbetare | Silver/tick L1 / L3 | Bruttokohorter L1 / L3 | Garnisonskohorter netto L1 / L3 |
|---|---:|---:|---:|---:|---:|---:|
| Nuvarande | 4 (4–4) | 5 | 20 | 27,875 / 50,831 | 27,875 / 50,831 | 92,917 / 169,436 |
| Bördigare | 4 (4–4) | 10 | 40 | 55,75 / 101,662 | 55,75 / 101,662 | 185,833 / 338,873 |
| Fler, mål5 | 5 (5–5) | 5 | 25 | 33,625 / 61,316 | 33,625 / 61,316 | 112,083 / 204,387 |
| Båda, mål5 | 5 (5–5) | 10 | 50 | 67,25 / 122,632 | 67,25 / 122,632 | 224,167 / 408,775 |
| Fler, mål10 | 10 (10–10) | 5 | 50 | 66,125 / 120,581 | 66,125 / 120,581 | 220,417 / 401,936 |
| Båda, mål10 | 10 (10–10) | 10 | 100 | 132,25 / 241,162 | 132,25 / 241,162 | 440,833 / 803,873 |
| Fler, mål20 | 15,55 (11–19) | 5 | 77,75 | 103,625 / 188,963 | 103,625 / 188,963 | 345,417 / 629,877 |
| Båda, mål20 | 15,55 (11–19) | 10 | 155,5 | 207,25 / 377,926 | 207,25 / 377,926 | 690,833 / 1259,755 |
| Fler, mål40 | 15,55 (11–19) | 5 | 77,75 | 103,625 / 188,963 | 103,625 / 188,963 | 345,417 / 629,877 |
| Båda, mål40 | 15,55 (11–19) | 10 | 155,5 | 207,25 / 377,926 | 207,25 / 377,926 | 690,833 / 1259,755 |

**Per Wanax vid 10 respektive 50 spelare**, på samma 64×64-karta. Silver/tick-kolumnen är också antalet bruttokohortekvivalenter. /50 är en jämförelsenormalisering, inte bevis för att alla 50 får plats eller äger silver.

| Väg | Silver och bruttokohorter/Wanax vid10, L1 / L3 | Nettokohorter/Wanax vid10, L1 / L3 | Silver och bruttokohorter/Wanax vid50, L1 / L3 | Nettokohorter/Wanax vid50, L1 / L3 |
|---|---:|---:|---:|---:|
| Nuvarande | 2,788 / 5,083 | 9,292 / 16,944 | 0,557 / 1,017 | 1,858 / 3,389 |
| Bördigare | 5,575 / 10,166 | 18,583 / 33,887 | 1,115 / 2,033 | 3,717 / 6,777 |
| Fler, mål5 | 3,362 / 6,132 | 11,208 / 20,439 | 0,672 / 1,226 | 2,242 / 4,088 |
| Båda, mål5 | 6,725 / 12,263 | 22,417 / 40,877 | 1,345 / 2,453 | 4,483 / 8,175 |
| Fler, mål10 | 6,612 / 12,058 | 22,042 / 40,194 | 1,323 / 2,412 | 4,408 / 8,039 |
| Båda, mål10 | 13,225 / 24,116 | 44,083 / 80,387 | 2,645 / 4,823 | 8,817 / 16,077 |
| Fler, mål20 | 10,363 / 18,896 | 34,542 / 62,988 | 2,072 / 3,779 | 6,908 / 12,598 |
| Båda, mål20 | 20,725 / 37,793 | 69,083 / 125,975 | 4,145 / 7,559 | 13,817 / 25,195 |
| Fler, mål40 | 10,363 / 18,896 | 34,542 / 62,988 | 2,072 / 3,779 | 6,908 / 12,598 |
| Båda, mål40 | 20,725 / 37,793 | 69,083 / 125,975 | 4,145 / 7,559 | 13,817 / 25,195 |

Spridningen mellan kartorna är stor: baseline L1 **20–42,5**, L3 **36,471–77,5**; mål10/femplatser L1 **50–72,5**, L3 **91,176–132,206**; mål20/femplatser L1 **70–140**, L3 **127,647–255,294**. Tio platser dubblar alla dessa intervall. Mål20 och mål40 är identiska kartor: faktiskt **11–19 hex**, medel15,55. Kandidaterna tar slut under spacing5; ett större måltal i sig hjälper inte längre. Det är ingen generell optimal packningsgräns utan resultatet av dagens giriga placering på dessa frön.

Bemanningskostnaden är också verklig: 20 gruvgubbar äter 10 korn/tick, 50 äter25, 100 äter50 vid civilranson0,5. Samma personal kan inte samtidigt odla korn eller gjuta brons. Fördubblade platser fördubblar denna belastning vid full kapacitet; de skapar ingen personal.

## A — rättvisa och avstånd

Axialavstånd mäts utan wrap med `(abs(dq)+abs(dr)+abs(dq+dr))/2`, samma mått som SQL. **Fyra/åtta hex betyder närhet, inte tillgång i catchment**: catchment har radie2 och kräver annan grundningshex när staden inte kan arbeta centrum. Ett avstånd över hav är inte en framkomlig marschväg. Även "samma landmassa" kan innehålla flodbarriärer; det är en kontroll mot havsgenvägar, inte A*-bevis. Andelar är poolade över alla punkter i urvalet, så en karta med fler kandidater väger mer i kandidattabellen.

| Silvermål | Alla statiskt möjliga starthex | Median / max hex | Inom4 | Inom8 | Silver på samma landmassa | På samma landmassa inom8 |
|---|---:|---:|---:|---:|---:|---:|
| 4 | 17278 | 11 / 52 | 16,507 % | 38,401 % | 74,413 % | 38,193 % |
| 5 | 17278 | 10 / 52 | 20,431 % | 44,502 % | 78,898 % | 44,224 % |
| 10 | 17278 | 6 / 48 | 37,892 % | 66,101 % | 87,811 % | 65,8 % |
| 20 | 17278 | 4 / 45 | 53,658 % | 79,812 % | 87,811 % | 79,338 % |
| 40 | 17278 | 4 / 45 | 53,658 % | 79,812 % | 87,811 % | 79,338 % |

| Silvermål | Värdstarter | Antal punkter | Median / max hex | Inom4 | Inom8 | Silver på samma landmassa | På samma landmassa inom8 |
|---|---|---:|---:|---:|---:|---:|---:|
| 4 | Första10 | 200 | 15 / 46 | 31,5 % | 38,5 % | 44,5 % | 38 % |
| 4 | Upp till50 | 992 | 12 / 50 | 13,105 % | 35,484 % | 69,859 % | 35,181 % |
| 5 | Första10 | 200 | 12 / 46 | 37,5 % | 44,5 % | 49,5 % | 43,5 % |
| 5 | Upp till50 | 994 | 11 / 51 | 15,594 % | 39,94 % | 73,843 % | 39,537 % |
| 10 | Första10 | 200 | 5 / 40 | 49,5 % | 54 % | 56 % | 52,5 % |
| 10 | Upp till50 | 995 | 7 / 48 | 30,754 % | 60,201 % | 81,709 % | 59,397 % |
| 20 | Första10 | 200 | 2 / 36 | 54,5 % | 56,5 % | 55,5 % | 54,5 % |
| 20 | Upp till50 | 992 | 5 / 43 | 44,657 % | 72,984 % | 81,956 % | 72,077 % |
| 40 | Första10 | 200 | 2 / 36 | 54,5 % | 56,5 % | 55,5 % | 54,5 % |
| 40 | Upp till50 | 992 | 5 / 43 | 44,657 % | 72,984 % | 81,956 % | 72,077 % |

**Alla bördigare-varianter har exakt samma avstånd som motsvarande femplatsvariant.** Bördigare fyra hex har därför baseline-avstånden; båda/mål10 har mål10-avstånden, osv. Ingen av dessa kapacitetsändringar kan flytta en startpunkt.

Första tio kunde placeras på alla20 kartor. Vid försök till50 blev antalet **992/994/995/992/992** för mål4/5/10/20/40; lägsta antal per karta47/47/47/43/43, högsta50. SQL kan nå en girig återvändsgränd tidigare beroende på starter även när geografin är identisk. Saknade värdar räknas inte som lyckade/nära eller i en falsk denominator1000. Värdarna flyttar eller grundar inte här; verklig spelutveckling förändrar occupancy. Vaultens `megaron_kartstorlek_alfa_20261012.md` varnar redan för att64×64 är trångt för50. Den frågan kräver eget befolknings/kartbeslut; /50-kapaciteten här är fortfarande enligt ordern.

### Vad styr placeringen, och kan den garanteras?

`world/mapgen.go:480,497` räknar tänkta spelare från area och väljer `max(4,players/3)`. `:1643–1693` väljer kulle/kalkberg, filtrerar bort koppar/tenn, och lägger enhexkällor. `depositSources:1897–1941` går round-robin över kandidatlandmassor med deterministiskt sorterade komponenter och slumpad ordning inom dem. Spacing5 gäller globalt (`:449,1950`); `:1823–1839` tar bort kalkbergsfyndigheter utan grundningsbar hex inomradie2. `validateMap:761` har **ingen** kontroll att varje startkandidat ligger inom4/8 från silver och inget höjt silverantal som efterkrav. Därför reseedas inte en karta för att målet20 inte nåddes.

`pickSpawnTile` väljer först minst belastad **träförsörjd** landmassa, därefter metallscore: halvklotets koppar/tenn2 + silver1 inom framtida catchment1–2, därefter RANDOM. Klart från andra värdar/stadsprovinser kräver avstånd>4. Silverpreferensen kan inte besegra en mindre belastad silverlös ö. Den gamla `SpawnOreCatchmentScore` i world-paketet har annan radie och saknar silvertermen; den har därför **inte** använts i utredningen.

**Dagens antalsspak garanterar inte N=4 eller8.** Inte ens mättat mål40 klarar det: maxavstånd45 för möjliga starthex,43 för upp till50 faktiska starter; 44,5 % av de första10 har ingen fyndighet ens på samma landmassa. En garanti måste vara ett **nytt kontrakt**: antingen täck varje möjlig starthex med produktiv silverkälla inomN och validera/reseeda efter placeringen, eller begränsa spawn till en förutbestämd täckt startregion. Den senare ändrar nuvarande landmassbalans/strövande värd. Kandidatöar kan sakna ledig kulle/kalkberg; en-per-ö räcker dessutom inte för en lång ö. Man måste besluta om terrängkonvertering, spacingundantag, omseedning eller otillåtna startregioner om täckning inte går att få. Att lägga silver bara kring de första förberäknade10 punkterna är ingen garanti för nästa/50:e värd.

Historiskt valde Timothy handelsberoende och värdvandring framför tredubbling eller ore före landmassbalans (`megaron_silvergeografin.md`, 2026-08-28). Nuvarande order öppnar för fler; den ändrar inte redan beslutad spawnordning eller metallhierarki. Mål10 kan göra silver vanligare i hexantal än koppar på vissa kartor och behöver ett nytt uttryckligt kanonbeslut. Rekommendationen ändrar inte detta tyst.

## B — spjutkohortens ranson och sold

Endast spjutens silver sätts kontrafaktiskt till0; ranson, storleksskalning och engångsrekrytering behålls. Kornets standardreferens är1 dagsverke/enhet. Fältranson gäller marching/positioned/embarked; skepps ranson dubblas inte. Navykostnader gäller per skrov, landkostnader här per100 man.

| Typ | Korn/tick garnison / fält = dagsverken | Silver/tick idag → B | Silver/tick hemma netto idag → B | Rekrytering, korn + silver (oförändrat) |
|---|---:|---:|---:|---:|
| Spjut100 | 0,5 / 1 | 1 → 0 | 0,3 → 0 | 12 + 20 |
| Elit100 | 0,6 / 1,2 | 2 → 2 | 0,6 → 0,6 | 24 + 40; även16 brons |
| Stridsvagn100 | 0,8 / 1,6 | 3 → 3 | 0,9 → 0,9 | 40 + 50; även8 timmer,8 ceder,20 brons |
| Galär20 besättning | 0,04 / 0,04 | 1,5 → 1,5 | 0,45 → 0,45 | 0 + 6; även30 timmer |
| Krigsgalär50 besättning | 0,06 / 0,06 | 2,5 → 2,5 | 0,75 → 0,75 | 0 + 30; även100 ceder |
| Handelsskepp10 besättning | 0,03 / 0,03 | 1 → 1 | 0,3 → 0,3 | 0 + 2; även16 timmer |

Källor: `combat/upkeep.go:77–118`, `province/training.go:108–113`, `unit/model.go:60–70`. En spjutkohort kräver **182,5 dagsverken korn/365 ticks i garnison**,365 i fält och12 vid rekrytering. Efter grundning äter startens två kohorter1/tick hemma,2 i fält; detta försvinner inte med solden. Före grundning har värden i dag grainRate0 och lever på subsistens; B ska inte införa en ny grundarranson. Silver vid rekrytering20 är en egen kostnad, inte löpande sold; att också ta bort den vore ett separat beslut. Linear B-ransonkopplingen är orderns kanonmotiv, inte ett nytt historiskt belägg framtaget här.

### Varje läsare och följdkedja

Sökningen finns i [upkeep-readers](upkeep-readers.txt), [ytor](upkeep-surfaces.txt). Produktionsläsarna av den centrala spjutsolden är uttömmande enligt `rg UnitUpkeep/UpkeepSpecs` på basen; testläsare är inte produktionskostnader.

| Läsare/led | Källa | Följd av Silver0 |
|---|---|---|
| Spec och storlek/status | `combat/upkeep.go:79,100–118` | Spjutsize/100 ger0 silver; grain lever kvar. Okända typer har noll båda, spjut har inte det. |
| Tickdebitering/payer | `upkeep.go:315–329,385–418,489–556` | Spjut får grainattrition som förut. `silverNeed<=0` hoppar över silverspärr, cirkulation och betalning. Payer måste finnas/ägas för mat; grundarvärdens aktiva units undantas från denna tick. |
| Afford-gate och sold-share | `upkeep.go:528–541` | Fulla `stock>=N` krävs fortsatt för yrkesförband; spjuten når inte grenen. 0,7 återbäring ×0 är0, ingen dold mintning. Discriminator är `settlement_id=payer`, inte bara status. |
| Obetalt och desertion | `upkeep.go:552–556,929–1009` | Ingen ny spjutsilvervarning/desertering. Befintliga `unpaid_periods` **nollställs inte** eftersom zero-silver-grenen fortsätter före paid-reset. Kräver explicit övergångsbeslut/rening, utan att återuppliva döda förband. Matbrist och annan deserteringsorsak ska finnas kvar. |
| UpkeepSettled/SilverAudit | `combat/silver_audit.go` + `upkeep.go:548–596` | Spjut bidrar inte längre till silverGross/unitsPaid/circulated/destroyed; grain kan fortfarande emitteras. `units_paid` är silverbokföring, inte antalet matade. Gamla event läses oförändrat. |
| Förhandsafford/sustainable | `api/handlers/province.go:393–404` | Ny spjutupkeep0 kan vara silversustainable även utan gruva. Rekryteringens20silver och byggnads/populationsgate finns kvar. |
| Garnisonslista | `province.go:1010–1042` (`armyUpkeep`) | Total och perType visar0 silver för spjut, rätt grain. Detta är composition här, inte payer-netto. |
| Stadens drain/projektion | `province.go:1062–1092,1107` (`settlementUpkeepDrain`, `upkeepNetPerDay`) | Stadens spjutnetto0; professionellas sold-share kvar. Nuvarande projektion saknar repairing/freighting som tick kan debitera: befintlig verifieringslucka, inte en B-fix. |
| Efter rekrytering | `province.go:2683–2693` | Varningens spjutsilver0. Nyenhet jämförs idag mot brutto, medan förhandsytan använder netto: befintlig skillnad för professionella, VERIFIERING. |
| Nyvärd och Rise | `api/handlers/nomadic_host.go:171–180`, `rise.go:23` | Ny seedrate−2→0, seedpung480→0; Rise en spjut×60ticks blir60→0. **Redan lagrad founder_phase.silver_rate ändras inte av tabellen.** Övergång behöver settle/bevara gammal balans och sätta ny rate0, ingen raderad valuta. |
| Grundarens resurspanel/grundning | `api/handlers/found_metropolis.go:169,556–601` | Läser lagrad rate och balance. Ny zero-rate visas som oändlig/ingen soldnedräkning; gamla rate kräver backfill. Vid grundning går saldo in i MetropolisSilver en gång. |
| Kolonistpung | `combat/march_start.go:868,1258–1259` | Full spjutpung30→0, elit60 och vagn90 kvar. Befintlig `units.carried_silver` får inte nollas; den återbetalas/flyttas som befintlig valuta. |
| Kolonins silverstock/cap | `combat/unit_arrival.go:2001`, `economy/genesis_silver.go:26` | Capminimum60 finns kvar trots noll pung; aldrig cap under befintligt settled stock. Ingen extra genesiskran. Behåll30 som separat startbuffert bara efter beslut. |
| VoyageRation | `upkeep.go:134,136` | Läser bara grain från samma funktion. Embarkerad spjutsilver0 förändrar inte fartygsprovianten. |
| UpkeepSpecs direkt för sjöfrakt | `combat/standing_orders.go:97` | Läser bara merchantman.Grain; ingen spjutsilverläsare. |
| Keryx | `cmd_status.go:776,831,1007–1021`, `cmd_recruit.go:243–273,333`, `cmd_notifications.go:657`, `cmd_founding.go:29,62` | API-siffror följer specen; visa ranson/sold0 och professionell sold. Grundarens ticks_left saknas vid zero-rate. Historiska obetalda events ska fortsatt återges. |
| Megaron | `web/static/js/megaron/ui/format.js:413–421`, `render/map.js:3565,3626` | Historiska unpaid/deserted ska återges, nya spjut ska inte få silvervarning. Ingen oberoende spjutsoldkonstant hittades i JS. hostStoreLine visar idag "Escort pay lasts indefinitely" vid saknad ticks_left; framtida B behöver en begriplig text för ingen sold. |
| Codex (framtida B kräver ändring) | `web/static/codex/{upkeep,units,silver,horde,colonies}.md` | "Every soldier draws pay", värdens pay-clock och kolonins30 kopplade till spjut blir fel. Artiklarna lämnas orörda under denna utredning. |

**VERIFIERING:** Nuvarande `UpkeepHandler` undantar alla aktiva founder units; övrig kodsökning visar inget separat founder-silverdeserteringsförlopp. Codex säger att eskorten deserterar vid slut pung. Förändra inte B kring ett obevisat sådant förlopp; en riktig värd-pungslutstest behöver avgöra befintlig drift först. Samma audit-luckor/founder money och AI-guvernörens avdrag från silverkalibreringen är fortsatt utanför scope.

### Startkassa: faktisk kod och nytt beslut

Metropolens belopp är idag **konstanter**, inte en dynamisk365×sold-formel (`economy/genesis_silver.go:8–21`). Att ändra enbart specen lämnar730/1278 och caps1460/2556 kvar; den beslutade *motiveringen* "ett års faktisk värdsold" upphör. Om man i stället räknar om efter B skulle inland bli0, kust `ceil(365×1,5)=548`, med cap0/1096 om gamla dubbleringsmotivet används. Noll inland stänger bland annat första spjutrekryteringens20silver och silverbönerna3/2/3/3 tills gruva/handel nås; Walls har ingen silverkostnad.

**Fråga till Timothy, rekommenderat alternativ:** behåll730/1278 som uttryckliga startbuffertar för rekrytering, böner och framtida professionella, med oförändrade caps och `max(kvarvarande, buffert)` vid första grundning. De är inte längre "ett års spjutsold". Med730 kan man finansiera en full elit i365 ticks brutto om man bortser från rekrytering; rekryterar man för40 blir saldot690 och runway345 ticks, eller1150 hemma med netto0,6. Kustbufferten1278 täcker den kvarvarande galärens852 ticks brutto, alltså betydligt mer än365. Det är en ändrad generositet som måste sägas högt. Låt även Timothy välja om Rise60 och spjutkolonins30 ska frikopplas som buffert eller följa solden till0. Inget nytt silvergolv, ingen extra återgrundningskran eller pop/kornformel föreslås.

### Hur stor del av efterfrågan försvinner?

[payroll.py](payroll.py) läser dagens `UpkeepSpecs`, kombinerar med dev-snapshot och skapar [payroll.json](payroll.json). Dev: en inlandsvärd, två fulla spjut, lagrad founder-rate−2/pung480, inga professionella eller grundade arméer. **Nominell löpande sold2→0, minskning100 %.** Det är en kontrafaktisk skyldighet; befintlig lagrad rate fortsätter tills migrerad. Det bevisar inte historiskt faktiskt betald sold eller total silverefterfrågan. SilverAudit missar founder-/carried-/transportstock och saknar ett tillförlitligt helt flöde; engångsrekrytering, riter och annan efterfrågan ingår inte i denna procent.

Följande är uttryckliga arméscenarier, inte observerad population. Netto antar att alla står hos egen betalare och defaultshare0,7; full fältsold är brutto. Mixar med olika garrison/field lägen får annan nettominskning.

| Armé | Sold/tick brutto före → efter | Hemmanetto före → efter | Spjutandel av brutto |
|---|---:|---:|---:|
| Inland:2 spjut | 2 → 0 | 0,6 → 0 | 100 % |
| Kust:2 spjut + galär | 3,5 → 1,5 | 1,05 → 0,45 | 57,143 % |
| 2 spjut + elit | 4 → 2 | 1,2 → 0,6 | 50 % |
| 10 spjut + elit + vagn | 15 → 5 | 4,5 → 1,5 | 66,667 % |
| 2 spjut + en av varje professionell typ inklusive de3 skeppstyperna | 12 → 10 | 3,6 → 3 | 16,667 % |

## Samspel A och B — dimensionera efter de yrkesförband som faktiskt ska finnas

För10 respektive50 inlandstarter faller spjutsolden från20/100 brutto (6/30 hemma) till0/0. För alla kuststarter faller35/175 till15/75 brutto, hemma10,5/52,5→4,5/22,5. Baseline medel27,875 räcker matematiskt till10 kustgalärer eller50 hemma efter B, men inte till50 galärer i fält. Baselines svagaste karta ger20: den räcker inte ens till50 hemma (22,5), utan buffert/annan supply. Mål10 ger minst50 på L1: täcker50 galärer hemma, men inte75 i fält; minst91,176 på L3 täcker dem i fält. Allt kräver allgruvornas bemanning och handel, inte att varje kuststad får egen gruva.

Vid mål10/femplatser kan världens medeloutput66,125/120,581 bära följande professionella förband, **alternativt, inte samtidigt**:

| Typ | Full bruttosold: förband totalt L1 / L3 | Egen betalare: nettoförband totalt L1 / L3 |
|---|---:|---:|
| Elit, sold2 | 33,063 / 60,290 | 110,208 / 200,968 |
| Vagn, sold3 | 22,042 / 40,194 | 73,472 / 133,979 |
| Galär, sold1,5 | 44,083 / 80,387 | 146,944 / 267,958 |
| Krigsgalär, sold2,5 | 26,450 / 48,232 | 88,167 / 160,775 |
| Handelsskepp, sold1 | 66,125 / 120,581 | 220,417 / 401,936 |

/10 och/50 ger perWanax; återigen ojämn faktisk fördelning. Exempel50 Wanax, varsin elit:100 brutto/30 hemma; mål10 medel räcker bara hemma på L1 och brutto på L3, vars svagaste karta91,176 ändå **inte** räcker för100 brutto. Varsin elit+galär kräver175 brutto/52,5 hemma: mål10L1 medel räcker hemma men minimum50 gör inte det; L3minimum91,176 räcker hemma, inte brutto. Dessa konkreta val avgör behovet av både antal och platser. Om B väljs finns inget stöd att dubbla gruvplatser bara för de två spjutkohorternas skull.

För en startarmé efter B behövs0 gruvarbetare för inlandets spjutsold, medan två kohorter fortsatt behöver1 korn/tick hemma. Kustens galär kräver1,5 brutto/0,45 netto: heltalsbemanning **2 kulle-L1-gubbar brutto eller1 netto**, alternativt1 kalkbergsgubbe för båda. Vid L3 räcker1 kulle-/kalkbergsgubbe även brutto. Det visar varför B gör mängdfrågan mindre akut men lämnar handel, elitkostnad och geografisk rättvisa relevanta.

Rekommenderad ordning för beslut: B och dess uttryckliga buffertar först; sedan A=fler mål10/femplatser som begränsad tillgångsförbättring, utan rättvisegaranti. Om målet istället är att **alla** hittar silver inom8 krävs ett särskilt täckningskontrakt och en ny mätning. Båda-spåret132,25/241,162 vid10hex/tio platser dubblar arbetsstyrkan till100 och gynnar redan silvernära spelare; välj det först om avsedd professionell armémix kräver det.

## Alla20 frön

Effektiva frön är identiska mellan samtliga varianter. L1 nedan gäller fem platser; tio platser ger dubbelt, L3 är varje cell×31/17. Mål40 är identiskt med mål20.

| Begärd seed | Effektiv | L1 mål4 | L1 mål5 | L1 mål10 | Faktiska hex mål20/40 | L1 mål20/40 |
|---|---:|---:|---:|---:|---:|---:|
| 1009 | 1011 | 27,5 | 32,5 | 65 | 17 | 100 |
| 2018 | 2018 | 27,5 | 40 | 65 | 14 | 92,5 |
| 3027 | 3029 | 42,5 | 47,5 | 72,5 | 15 | 112,5 |
| 4036 | 4036 | 20 | 25 | 65 | 14 | 92,5 |
| 5045 | 5046 | 20 | 25 | 57,5 | 19 | 117,5 |
| 6054 | 6055 | 35 | 40 | 72,5 | 16 | 117,5 |
| 7063 | 7065 | 27,5 | 32,5 | 65 | 11 | 70 |
| 8072 | 8073 | 20 | 25 | 57,5 | 18 | 105 |
| 9081 | 9084 | 35 | 40 | 72,5 | 17 | 122,5 |
| 10090 | 10092 | 27,5 | 32,5 | 72,5 | 17 | 107,5 |
| 11099 | 11101 | 27,5 | 32,5 | 72,5 | 15 | 112,5 |
| 12108 | 12109 | 20 | 25 | 50 | 15 | 90 |
| 13117 | 13118 | 27,5 | 32,5 | 65 | 16 | 102,5 |
| 14126 | 14126 | 27,5 | 32,5 | 72,5 | 12 | 82,5 |
| 15135 | 15135 | 20 | 25 | 50 | 15 | 90 |
| 16144 | 16144 | 27,5 | 40 | 65 | 14 | 92,5 |
| 17153 | 17153 | 27,5 | 32,5 | 65 | 15 | 105 |
| 18162 | 18170 | 35 | 40 | 72,5 | 16 | 110 |
| 19171 | 19172 | 35 | 40 | 72,5 | 19 | 140 |
| 20180 | 20182 | 27,5 | 32,5 | 72,5 | 16 | 110 |

## Bevis och stopp

Oberoende kontroll beräknar axialavstånd på nytt för varje startpunkt direkt från silverkoordinaterna, kontrollerar samma-landmassa/null, unika kandidater, start- och silverspacing5, och rater direkt från L1-ankaret/0,7-nivåformeln. Baseline matchar STOPP1. Två fristående fullkörningar100 kartor vardera har samma bytes för alla kartor/startpunkter. En fysisk mutation av det **isolerade analysskriptets** L3-faktor till1 blev namngivet röd `worker-rate anchor L3`; en manipulerad avståndspunkt blev röd `nearest-silver geometry`; oförändrade original blev färskt gröna. Dessa är analysbevis, inga produktionsmutationer eller naturliga spelprov.

**VERIFIERING kvar:** naturlig gruvbemanning/mat/handel, professionell armémix i alfa, storlek för50 spelare, hav-/flodfärdvägar och faktisk catchmentåtkomst, projektionens navalstatus/bruttovarning, grundarens påstådda silverdesertering, auditens ofullständiga valutasystem och AI-guvernörens tysta avdrag. Inga sådana luckor har byggts eller dolts under A/B.

**STOPP.** Claude granskar och tar kanonfrågorna till Timothy. Ingen implementation, merge, push eller deploy här.
