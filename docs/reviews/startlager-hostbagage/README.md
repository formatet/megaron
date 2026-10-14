# Startlager från hostbagage

## SAMMANFATTNING
Nya grundningar flyttar bara hostens kvarvarande korn och silver; extra 730/1278-silver och 8-kornsfrö är borta. Kolonin får bara bärd börs och inget kornfrö. Ny host 1200 korn/480 silver, Rise 60, befintliga saldon och separata boskaps-/gudagåvor bevaras.

Produktion **6935e404**, rebased på **6bd50597** (hostfart och nådefrist). Full färsk Go på PG179 grön, vet ren, 623 JS gröna. Tre fysiska mutationer röda→exakta bytes→färsk grön. Två färska naturliga provvärldar med inland + kust: CLI-förhandsvisning/grundning och Firefox 1280/390 visar 1200 korn/480 silver; silvercap 960, timmer/sten 0. `native-repeat` är slutligt bildpaket, med normalt stängd introduktionsbrief och både 480 och 1200 synliga.

**LIVE enligt Claudes integrationskvitto 2026-10-14 22:03:** merge **4e2e8bd0**, ny process `healthz {commit:4e2e8bd,migration:179,status:ok}`. Claude körde dessutom egen full färsk Go/vet/623JS och återinfört silvergolv730→fyra namngivna röda→återställt grönt. Detta är Claudes driftkvitto, skilt från Codex egna acceptansprocesser: båda `native/verification.json` och `native-repeat/verification.json` bär **health6935e404/179** och tom produktionsdiff. Denna avslutande commit innehåller enbart docs och cherry-pickas av Claude; inga fler produktionsändringar. Kodjämförelsen `git diff 6935e404 origin/master -- server web tools` var tom vid rapportavslut: acceptansens produktionskällor är desamma som integrerade master.

Risk: 480 köper fortfarande rekrytering/böner, men silver kan inte ersätta byggmaterial eller bronsgrinden. 10 boskap behålls enligt Claudes besked; eventuell ändring kräver Timothy och står i BESLUT.

## SLICE-KONTRAKT
| Fält | Kontrakt |
|---|---|
| Problem | Grundning skapar extra silver/korn som hosten inte bar. |
| Spelarsanning | Hostens kvarvarande grain/silver går in exakt en gång; kolonin får bärd börs och0extra grain/silver. |
| Invariant | Gamla städer/världar/historiska events, host1200/480, Rise60, kolonipung30/60/90 och Demeterfarm/Poseidongalär bevaras. |
| Scope | Grundningssaldo/cap, grainfrö, prognos, relaterade tester/CLI/webb/Codex/tools/vault. |
| Non-scope | Ingen migration, ändrade gudapriser/gruvor/recruit/build/prayerkostnader eller annan mekanik. Boskap10 bevaras enligt Claudes besked; framtida kanonfråga är parkerad. |
| Acceptans | Inland/kust/refugee exact bagage; kolonibörs och0grain; forecast=saldon på alla ytor; fysisktopupmutation; fullfreshGo/vet/JS och naturlig CLI+Firefox1280/390grundning. |
| Stopvillkor | Kanonfrågor i BESLUT, fortsätt oberoende delar. Claude granskar/integrerar. |
| Bevisplan | Oförändrad baslinje, faktisk HTTP/PG-grundning och colonyarrival, forecastpayload, naturlig register/join/found/CLI/browser, mutation. |

Arbetet **bevisar** kedjegrindens geografi→brist: startlagret visar faktisk bärd försörjning och den nya stadens produktion ska täcka fortsatt behov.

## BASLINJE
`baseline.log`: oförändrad 3c1addc1, separat tom PG179, silvergrundning/refugee/audit/dowry-prov gröna. Tidigare 730/1278-topup och kapital/koloni8korn. WIP bevarades i d545deab och rebase på aktuell6bd50597 gav6935e404. `affected.log` provar faktisk grundning med spent/full/high-purse och decimalgrain, refugee/Rise, direkt createMetropolis och verklig kolonigrundning via arrival.

## EXPERIMENT
| Ändring | Resultat | Beslut |
|---|---|---|
| Alla kapitalers silver = max(0,buren börs), ingen eligibilitygren | Inland/kust7→7,480→480,5000→5000; direkt caller utan börs0, refugee17→17 | Behåll |
| Cap från samma purse-regel som koloni | max(60,2×börs,stock):7→60,480→960,5000→10000; aldrig under börsen | Behåll; inget saldo skapas av cap |
| Kapitalgrain0 och ColonyGrainSeed0 | Kvarvarande17,25/1200grain flyttas exakt; refugee93,5 bevaras; koloni0grain och exakt silverbörs | Behåll |
| CLI:s gamla extra-startlagertext; CLI/webb trunc/round av grainparam | Receipt förklarar att inget extra skapas;17,25 behålls i verklig HTTP-förfrågan och svar | Paritet, inga nya features |
| Gamla silvergolvet730 fysiskt tillbaka | `TestSilverCalibrationFirstFoundingCarriedOnly` rött; exakta bytes tillbaka, separat freshPG179 grönt | `silver-topup-*` |
| Kapitalgrain8 fysiskt tillbaka | Samma riktiga grundningsprov rött; byteåterställning + fresh grönt | `capital-grain8-*` |
| Kolonigrain8 fysiskt tillbaka | `TestFoundColony_DoesNotMintSilver` rött på grainassertion via arrival; byteåterställning + fresh grönt | `colony-grain8-*` |

ColonySQL `$2` är **economy.ColonyGrainSeed**, ett rent frö, inte last/kargograin. Det är nu0. Silverbörsen flyttas separat atomiskt och töms på unit. Kapitalens extra8korn är0; hostgrain läses settled och läggs till en gång. `foundedMetropolis.GrainTotal/SilverTotal` och lagrade saldon kontrolleras, inte bara formatterad text.

## VAD 480 SILVER RÄCKER TILL — KOSTNAD OCH RISK
Faktisk katalog ur acceptansprocessen: [cost-catalogues.json](native-repeat/cost-catalogues.json), `GET /api/v1/buildings` och `/api/v1/units`, med batchstorlek/gates. Referenser i kod: `province.BuildingSpecs`, `province.UnitSpecs`, `unit.CrewFor`, `religion.PrayerSpecs`. Inga handelspriser antas.

| Köp/åtgärd | Silver | Övriga kostnader och grinden |
|---|---:|---|
| 100 Spearmen |20|12korn, barracks;1tick träning |
| 100 Elite Infantry |40|24korn+16brons, barracks+foundry;4ticks |
| 100 War Chariot |50|40korn+8timmer+8ceder+20brons, stable;3ticks |
| Galley (20crew) |6|30timmer, shipyard;3ticks |
| Emporos (10crew) |2|16timmer, shipyard;4ticks |
| War Galley (50crew) |30|100ceder, shipyard+foundry;5ticks |
| Potnia's Vision |3|10korn, tempel/Kharisgate; lyckad böns väntetid24ticks |
| Green Shoots |2|15korn, tempel/Kharisgate; lyckad böns väntetid25ticks |
| Thread Shows a Path |3|4timmer, tempel/Kharisgate; lyckad väntetid12ticks |
| Favourable Passage |3|6fisk, tempel/Kharisgate; lyckad väntetid12ticks |
| Mistress of the Walls |0|8korn+4timmer, tempel/Kharisgate; lyckad väntetid24ticks |

Cash-only tak är24 spjutkohorter,12 elitkohorter eller9vagnskohorter; det är **inte** vad1000invånare och startresurser kan rekrytera. Arbetskraft, byggnader, brons och andra material begränsar först. En spjutkohort20 och Vision3 lämnar457silver, men båda kräver först sina respektive byggnads/Kharisgates. En3-silverbön är0,625% av480, Green Shoots2 är0,417%; startkassan är inte den första bönegrinden.

L1-byggnader tar **ingen silverkostnad** i katalogen. Barracks8timmer+16sten/10ticks, temple8timmer+24sten/16ticks, mine6timmer+16sten/10ticks, lumbermill8timmer+4sten/6ticks och stonequarry6timmer+8sten/6ticks. Med timmer/sten0 kan en ny stad inte köpa någon av dem direkt; den måste placera gubbar i catchmenten och producera material. Silver fungerar inte som NPC-byte till saknade material. Detta förblir geografi→brist-grinden. Demeterfarm/Poseidongalär är de befintliga separata gåvorna.

Den första stadens kassa kan alltså inte köpa en ny byggnad eller kohort omedelbart: inga timmer-/stenlager eller barracks/shipyard/temple finns ännu. Efter materialinsamling upptar barracksbygget 10 ticks och första spjutträningen ytterligare 1; templet tar 16 ticks efter att dess material finns. Kharis-/offergrindar tillkommer. Det är villkorade mekaniska tider, inte en uppmätt tid från join för alla geografier.

**Första ticks:** inland utan professionella har ingen daglig spjutsold och480 är därmed kvar tills order/offergåva/rekrytering spenderar den. Kustens gratis galär har1,5silver/tick brutto i aktiv kontext; enbart det ger320ticks av480, före nyrekrytering/böner och före garnisonsåtercirkulation. Soldens netto kan vara lägre hemma; rapporten använder den försiktiga bruttoramen, inte en ny payrollregel. Professionella har fortsatt2/3silver per tick för elit/vagn; en stad som raskt rekryterar sådana och skickar ut dem behöver gruva/handel/bärd försörjning.

För en full480börs är bufferten250silver mindre än tidigare inland730 (−34,2%) och798mindre än tidigare kust1278 (−62,4%). Därför blir professionell expansion och sjöflotta känsligare för treasury innan silverhandel/gruva finns. Rapporterad risk, inte åtgärdad med ett nytt golv. Kapitalens korn1208→1200 minskar däremot med0,66%; host1200/rate0 och auto-placering bevaras. Kolonins8→0 tar bort hela dess lilla matbuffert: ett dåligt valt catchment eller en mattransport som kommer sent kan ge tidig svält. Produktion/import behövs från början. Ingen koloniproviantfeature införs här.

Gudarnas **arbetsreferens** kommer fortfarande ur `goods.labor_value`, oberoende av startkassan: [value-independence.txt](value-independence.txt). Inga ändringar i religionens offerrecept, arbetsvärdemigration eller gruvankare. Den redan befintliga dynamiska knapphetsvärderingen i `religion/index.go` läser världens varulager och kan reagera på mindre silverstock; det är en annan sak än den fasta dagsverkesreferensen. `tools/alfa_economy.py` läser faktiska saldon/audit och hårdkodar inga starterkassor.

## GRINDAR OCH BILDER
| Grind | Bevis |
|---|---|
| Kod | `full-go.log`: hela färska PG179-sviten, world139,371s. `vet.log` ren; `js.log`623/623,0skip. |
| Funktion | `affected.log`, tre mutationspar enligt tabellen; varje arm separat tomPG179, byteidentisk återställning. SilverAuditV2 mäter480→480/netto0vid flytt, inte en skapad250topup. |
| Semantik/fyra ytor | Server/DB faktiskt bagage; CLI receipt + fractionseed-HTTP-test; webb verkligt forecastrequest1200 + lagerrader; Codex `silver`/`colonies`. Koloniforecast seed0från samma källa som arrival. |
| Användare | Normal register/join→riktig canvasclick på host→forecast→CLI `founding settle --yes`→API Goods + Firefox Goods. Inland/kust i varje privat ny värld, ingen SQL-fixtur/spelskrivning eller HTTP/CSS/payloadoverride. |
| Visuell |1280×900 och390×844 vid1:1. Slutliga Goods-bilder visar480 och1200på samma skärm efter normal brief-dismiss. Ingen layoutändring. |
| Bevarande/drift | Ingen migration/backfill/startupändring. Gamla saldon/vägar/eventsemantik orörda. Claude integrerade4e2e8bd0och verifierade nyhealth/mig179; egen driftkvittering se sammanfattning. |

Granska i **native-repeat/**: `inland-{desktop,mobile}-before.png`, `coast-{desktop,mobile}-before.png` (Host/catchmentforecast), `inland-{desktop,mobile}-after.png`, `coast-{desktop,mobile}-after.png` (480silver,1200grain,0timber/stone,10livestock). Motsvarande CLI-receipts är `*-keryx-founding.txt`; råpayload/trials/goods och health i `verification.json`. `native/` är första rena körningen; dess nya Economy-brief är synlig. Det slutliga omprovet stänger den genom riktiga UI-knappen så lagret ryms utan brief. TICK_SECONDS3600 håller snapshotens tick0; tiden läses ur riktig process, ingen produktionsmiljö ärvs.

Prognos=utfall avser bärd seed/saldo exakt. Foodnet/gross har olika betydelse: kolonize/Hostprognosen beräknar netto, Goods visar råproduktion; befintligt `TestFoundingGrainForecast_MatchesRealFounding` provar samma produktionsformel. Kust kan ha negativt **grain**net men positiv totalmat via fisk; rapporten behandlar inte grainraden som hela försörjningen.

## INVENTERING OCH KVARLÄMNADE TRÄFFAR
[seed-inventory.txt](seed-inventory.txt), [vault-inventory.txt](vault-inventory.txt), [value-independence.txt](value-independence.txt) innehåller sökmönstrens radnummer. Sökningen begränsar kodtyper och långa rader, så glyphdata/base64 inte maskerar relevanta tal.

- `world.go` seeddefault är samma ColonyGrainSeed0och blir0utan ny klientformel. Host-klienter skickar kvarvarande grain; serverparametern tillåter decimaler. `opening_dispatch.go` läser1200/480från hostspec, fortsatt sant och inga extra stadsgåvor lovas.
- `capabilities_test.go` CASE-grain är explicit resursfixture, inte grundningsseed. `map.js`730är rendererns slumpfrö för träd, orört. `silver_audit_v2_test.go` uppdateras från topup/netdelta250till flytt/netdelta0; efterföljande5silver-gruvcap och övrig auditmatematik bevaras med motsvarande förskjuten bas485, median242,5.
- `tools/silver_calibration_live.py` uppdateras till bärd receipt/179i sina assertions. `silver_calibration_mutations.py` uppdaterar borttagna endowmentanchors till avsiktligt **återinfört**730/1278golv och refugee60, med nya namngivna gränsprov. Tal kvar i mutationskod är avsiktliga fel, inga runtime-regler. Dess övriga pris-/gruvmutationer bevaras. Historiska rapporter/screenshotkvittomaterial skrivs inte om.
- Hjord10, dess tester/forecastförklaring och Demeterfarm/Poseidongalär är separata befintliga gåvor: uttryckligen orörda enligt Claudes besked20:50. Beslutsraden rekommenderar behåll/generöst; Timothy avgör framtiden. Det gör inte grain eller silver till seed.
- Övriga8kornträffar är Walls-prayer, hex/gubbeplatser, yields eller explicita fixturelager, inte initial seed. Kolonipung30/60/90 och Rise60 bevaras, liksom alla arbetsvärden/kostnader/rekryteringsgrindar.
- Aktuell vault uppdaterades smalt: silverkalibreringens regel/metadata, balansspakarnas kassabeskrivning, extern ekonomisammanställningsstartsaldo, ”ur tomma intet”s borttagna frö/extra silver, beslutregister och verblista. Äldre B-kvitto730/1278står uttryckligen som dåvarande/ersatt. Religionens sammanställnings135-rad är det daterade äldre offerrundsunderlaget, inte nytt startlager; todo:s äldre samtal/LIVEhistoria ägs av Claude och lämnas. Tidigare handelsrapporter/tabeller med1 730är ordervolym/rapporthistoria. `megaron_plan_tysta_forluster`, delta/byggplaner och valtabellarkivets4,8korn beskriver produktion, inte8seed; researchhavsrapportens730är URLsuffix. Alla återstående listade träffar är redovisade och ligger utanför starteregeln.

## METODISK LÄRDOM OCH AVGRÄNSNINGAR
En grundningsprognos måste skicka hela bärda mängden; annars kan samma serverformel ändå ge olika utfall i två klienter. En cap är utrymme och får inte blandas ihop med mängd. Säkerställ också att ekonomins audit mäter en förflyttning som netto0 när en gammal faucet tas bort.

Native bevisar nya naturliga inland/kustgrundningar och klienternas riktiga läsytor; spent/refugee/decimalbagage och colonyarrival är kontrollerade PG-fixturprov, inga påståenden om naturligt genomspelad kollaps/kolonimarsch i denna slice. Gamla städers saldon ändras inte av denna koddiff; ingen livemassfix körs. Ingen funktion eller extra balansändring utanför ordern införs.

Repro: `tools/gotest.sh`; ren `go vet ./...` i server; `node --test $(rg --files web/static -g '*.test.mjs')`; `python3 docs/reviews/startlager-hostbagage/mutate.py`; `STOCK_PROOF_OUT=native-repeat python3 docs/reviews/startlager-hostbagage/live.py`.
