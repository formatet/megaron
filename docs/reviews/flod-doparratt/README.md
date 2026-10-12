# Ortnamn — generellt doparrättssubstrat

## Sammanfattning

Timothy ville att spelare skulle få **döpa** landet; den gamla flodlistan gav färdiga namn. Gren `codex/flod-doparratt` ersätter listan med `world.Places`, en `place_names`-tabell, ett verb `name_place` och en notiskind `PlaceFirstReached`. Ursprung c71bf82f, rebasad på **origin/master 0db368c5**, inklusive bb49ef0d. Aktuell Go-produktionscommit **46e8f716**, Codex-regel **3c81038e**, vald etikett-/frontendimplementation **4820b9f7**. Inte mergat/pushat/deployat av Codex.

**Massivkanon avgjord och implementerad:** Timothy 2026-10-12 06:22 valde den första egna fysiska enhet som går in på mark direkt intill massivet. Berg förblir oframkomliga; pathfind är orört. En grannmark kan ge rätt till två separata massiv. **Etikett A är vald:** Timothy valde kursiv A och B/växeln är borttagna. **HOLD för nya BILD:** Timothy dömer A:s förbättrade kontrast i LIVE-sikt före merge; Claude granskar och integrerar grenen.

Tidigare bild-/flodriggprovenance (före massivtillägget): [processraden](provenance.txt), **a14f611 · migration 172/172 · värld 3b5fb9cd · tick 2810 · TICK_SECONDS=6 · 2026-10-12 00:40**. Världen fortsätter ticka. [Runtime/binary-SHA](runtime.json), [aktuella branch-assets](served-assets-final.json) och [bygget](rig-build-final.log). Lokal CGO=0-server på cachad isolerad acceptansruntime med aktuella migrationer/branch-assets, samma dokumenterade DNS-fallback som tidigare slicer; inget påstående om ett rent Dockerbygge. Endast `megaron-acc`/8097 används.

## Slice-kontrakt

| Fält | Kontrakt |
| --- | --- |
| Problem | Färdiga namn undanröjer spelarens doparrätt. |
| Spelarsanning | Första egna fysiska inträdet i objektet (massiv: grannmark) ger en bestående engångsrätt, synlig och användbar i webb/keryx. |
| Invariant | Synlighet ger ingen rätt; kind+ankare är stabilt; okända hexar läcker inget namn eller can_name. |
| Scope | Geografi, additiv migration 172, sparad-rutt-progress, generiskt API, CLI/webb/Codex/notis/Umami, vald kursiv etikett och nya LIVE-siktbilder. |
| Non-scope | Stormnamn, terränggenerering, rörelsebalans, gamla rörelsers backfill, budets nya ruttmodell. |
| Acceptans | Första inträde/omkörning; 403/409/400/unikhet/admin; FOW; fyra ytor; färsk helsvit/vet, fysiska mutationer, faktisk namnhandling och Firefox 1280+390. |
| Stopvillkor | Kanonkonflikt om fysisk nåbarhet lyfts till Timothy; inga bergs-/synlighetsregler smygs in. |
| Bevisplan | Baslinje från faktisk process, persistenta lifecycle-/köprov, riktiga namnhandlingar, nya LIVE-siktbilder. |

Kedjegrind: bevisar geografi-ytans handling och återkoppling; ändrar inte brons-/elitregler.

## Geografi, ägare och livscykel

| Kind | Statisk komponent | Minsta storlek |
| --- | --- | --- |
| river | river + river_ford; anslutna deltaöar ärver första kanoniska kedjan | Ingen |
| massif | mountain_limestone + mountain_red tillsammans | 5 hexar; fysisk rätt vid inträde på grannmark |
| desert | semi_desert | 5 hexar |
| forest | Endast forest_cedar; aldrig forest_olive_grove | 5 hexar |

Ankare = minsta `(q,r)` i grundkomponenten. Samma fulla statiska geografi används av server och rörelse; klienten skapar inga komponenter ur sin partiella FOW. Namn är skiftlägesokänsligt unika **över alla kinds i världen**. Namnreglerna följer stormarnas 2–30 tecken/filter/normalisering. Bara claimed_by får döpa en gång; admin kan sätta/byta/rensa med befintlig X-Admin-Key. named_tick bevaras efter rensning. Admin kan moderera en giltig statisk plats före spelaranspråket; ett senare inträde lovar då inte en redan förbrukad namnhandling.

Rörelseaudit hittade ingen gemensam DB-skrivning för varje hex: sparade rutter interpoleras, q/r skrivs vid ankomst/avbrott. **Claude beslutade 2026-10-12 00:12** att spegla stormscanens sparad-rutt/progress-mönster. En central `recordPlaceEntriesTx` i combat sorterar faktiska `RouteEnterMilli`-inträden över alla aktörer, låser resa/progress och skriver anspråk, händelse, notis och det verkligen besökta hexminnet atomiskt. För massiv behålls även en direkt angränsande bergshex som synligt vittne för notisens kartklick; hela det osedda massivet avslöjas inte. Ursprunghexen ger ingen ny rätt. Lika inträdestid bryts deterministiskt med resnyckeln. Progress har last_tick och last_milli för recall mitt i ett tick.

Täckt: **arméer, skepp och värden** med giltig aktiv sparad units.march_route; **landkaravaner och fraktskepp** med sparad transports.journey, oavsett transportärende. Fraktskeppets riktiga unit-id och kanoniska visningsnamn bevaras i notisen; karavaner använder transport-id. **Budbärare undantas uttryckligen enligt Claude:** ingen sparad hexrutt; de får inte en påhittad rutt eller synlighetsrätt. Gamla/ogiltiga NULL-rutter, orörliga garrison/positioned och förstörda aktörer räknas inte som nya inträden.

Återkommande PlaceReachScan ligger vid prioritet 7, efter stormförluster och före ankomst. Bara prioritet räckte inte vid catch-up: EnqueueTickRecurring hoppar till aktuell världstick+1, medan en äldre ankomst redan kan ligga i samma hämtade batch. **BeforeArrival** flushar därför samma centrala TX före Unit/Transport/Logistics/Trade/Gift/TradeReturn/SentryReturn/PickupTimeout. **Namngivning får aldrig stoppa rörelse (Claudebeslut 2026-10-12):** oläsbara resor loggas och hoppas över utan progressflytt. Vid DB-fel i flushen loggas felet och ankomsten fortsätter; just den doparrätten kan gå förlorad. Ingen ändring av events.Worker eller andra scans. Recall/redirect flushar gamla faktiskt inträdda hexar i samma ruttbytes-TX före bytet, med gemensam världs-/radlåsordning.

Ingen backfill: migrationen fångar befintliga världars nästa tick och tillämpningstid. För redan pågående resor hoppas deployens partiella tick över; framtida inträden räknas. Nya avfärder efter deploy räknar sitt första omedelbara steg. Ny värld har ingen historik att hoppa över. Förstörda aktörer hoppas över helt (Claudebeslut), även ännu oregistrerade inträden före förlusten; inget efter förstörelse kan ge rätt. Detta är inte en rekonstruktion av förstörda resors historia.

## Baslinje och fynd

[Färsk baslinje](baseline.log): oförändrad c71bf82f, migration 171, berörda geografi/stormnamnsprov gröna. [Verklig gammal process](baseline-health.json) var 4946d5e/171; [kända färdiga flodnamn](baseline-river-names.json) visar beteendet. Processhash och source-bas är medvetet åtskilda här, inte tillskrivna samma bygge.

| Hypotes/fynd | Ändring | Resultat |
| --- | --- | --- |
| Synlighet kan inte vara rättens ägare | Faktiskt inträde via sparad rutt, inget namnval ur CSV/hash | Riktiga DB-prov behåller första ägare och en notis vid omkörning; samma river/ford/delta ger en rätt. |
| Vanlig scanprioritet skyddar inte catch-up | BeforeArrival som tunn ankomstförutsättning | Riktig Worker.Run: scan due1 + ankomst due4 i samma batch, världstick10; sista hexen bevaras. Borttagen förutsättning ger namngivet rött. |
| Den första Host-enheten döljer vanliga terrängkortet | Namnrutan i Hostens befintliga scrollbody, grundningsknappen kvar i foten | Faktisk Firefox-namngivning fungerar; riktat inspect-prov bevarar founding action. |
| Ägd stads klick tar över hexpanelen, selectedHex rensas | Samma ruta i befintlig Catchment-sektion och reload av aktuell City efter namn | Verklig produktionstemplate-test visar handlingen och det nya namnet; förbrukad rätt försvinner. |
| Rå DB-typ är inte notisens igenkännbara subjekt | unit.LoadDisplayName, även för fraktskeppet | Kanoniskt Host-/fraktskeppssubjekt provas och arkiveras i nya notiser. Den första riggnotisen före fixen finns kvar som före-bevis. |
| /play tappar frågeparametern | Bilder laddas via faktisk /world/{id}/map?ortnamn=a eller b, med assertion på verklig URL | Två olika renderade varianter, inte två offlineberäkningar av samma bild. |

## Grindar och bevis

Aktuell kod: **587 JS-prov gröna**, färsk full Go och vet gröna. [full färsk Go mot46e8f716](massif/full-go.log), [full JS med vald etikett](live-labels/full-js.log), [vet](massif/vet.log), Python compile och diff --check. Tidigare överlämnandets 586-prov/fullsuite finns kvar som historiskt underlag. Riktade [API/FOW](final-focused.log), [rörelse/prioritet/G1](movement-focused.log), [verklig kö/ankomst/fraktskepp](arrival-focused.log), [Host/City-räckbarhet](panel-reachability.log). G1 placerar combat → world uttryckligen; stormarna är oförändrade.

Fysiska mutationer med återställd kod grön:

- [claimed_by IS NULL bort](mutation-first-owner.log): `earliest physical entry did not retain first owner`.
- [Behörighetsgrind bort](mutation-api-right.log): `nonclaimant naming ... want403`.
- [Centralt rörelseanspråk bort](mutation-movement-hook.log): `physical movement reach hook missing`.
- [Ankomstförutsättning bort](mutation-arrival-prerequisite.log): `catch-up arrival erased last reached hex`.
- [Webbrätt och läsbar CLI-notis](client-mutations.log), med separata återställda gröna loggar.

Användare/semantik: ParityNomads **vanliga API-marsch** från `(43,73)` till deltat `(41,74)`, ticks **2623→2626**, gav rätt till river **41,75**. Ingen SQL-ändring av den aktören, rutten eller rätten. [Sparad faktisk rutt](live-host-route.json), [anspråk/namn](live-host-claim.json), [rätt/notis efter ankomst](live-host-reached.json). Sedan återupptogs just den verkliga rätten i webb/keryx-provet: [scenario](live/scenario.json), [CLI före](live/places-before.txt), [efter](live/places-after.txt), [403/400](live/refusals.json), [andra försök 409](live/second-name-409.txt). Namnet **Doria** skrevs i riktiga Firefox-rutan och visas på kartan. Två nya normalregistrerade konton provades först men hade ingen nära känd namnlös komponent; ingen geografi/rätt injicerades för att göra dem gröna.

BILD: [naturlig Host-form 1280](live/desktop-natural-form.png), [390](live/mobile-natural-form.png), [döpt karta 390](live/mobile-natural-named-map.png). Hostens titel behåller aktören; namnet visas som egen geografiinformation när aktörens/stadens panel tar över. Terräng-/ruralkortets titel visar namnet utan upprepning på TERRAIN-raden.

## Massivets grannmark — kanonbeslut 06:22

Implementerat i **3c81038e**, ovanpå det tidigare överlämnandet 6f04dbf9. [Färsk oförändrad rörelsebaslinje](massif/baseline.log) var grön. Samma `recordPlaceEntriesTx` äger rätt, notis och progress för alla kinds. Endast massivets inträdesmål ändras till befintliga landframkomliga grannhexar, via oförändrade `province.IsPassable` och `hexgrid.Neighbors`. Vad räknas som landkorsning; hav och flodfåra räknas inte. Gemensamma grannhexar dedupliceras per massiv. Flod, öken och cederskog kräver fortfarande inträde i själva objektet.

[Riktade rörelseprov](massif/focused.log) visar grannmark → en rätt, grannmark intill två separata blandade femhexmassiv → två rättigheter/notiser, omkörning → inga dubletter, längre bort men ihågkommet → ingen rätt, hav/flodfåra → ingen massifrätt, vad → rätt. Notisen pekar på en ihågkommen angränsande bergshex; avlägsna bergshexar hamnar inte i minnet.

[API-livscykel](massif/api.log) börjar utan fixturens syntetiska rätt/minne och använder en fysisk sparad markrutt genom den riktiga scannen. Sedan tas den levande enhetens öga bort: katalogen visar fortfarande `can_name` och exakt en ihågkommen bergshex. Ägaren kan döpa, annan Wanax får 403 och återanvändning 409. Detta är ett DB-/API-integrationsprov, inte en ny riggexpedition.

[Fysisk mutation](massif/mutation.log) tar bort massivets grannmarksavbildning och ger namngivet rött `massif border entry rights=0 want1` (även tvåmassivfallet blir rött). [Återställd kod](massif/restored.log) blir grön. Reproduktion: `python3 tools/place_massif_mutation.py`.

Färsk full [Go](massif/full-go.log) och [vet](massif/vet.log) körda mot46e8f716; [JS](live-labels/full-js.log) körda med etikett4820b9f7 (samma frontend i46e8f716). Codex-artikeln *Naming the land* beskriver grannmark, oframkomliga berg och två massiv i spelarspråk. `git diff 6f04dbf9 -- server/internal/province/pathfind.go` är tom. Migration, schema, stormnamn och pathfind ändras inte i massiv-/ankomsttillägget. Etikettändringen redovisas separat nedan.

## Rörelse fortsätter vid namngivningsfel — granskning 06:32

**46e8f716** följer Claudes granskningsbesked. `BeforeArrival` loggar ett misslyckat namnflush och anropar ändå ankomsthandlern. Ruttfel loggas per resa; en ogiltig resa lämnar sin sparade progress orörd. `LoadActiveRoute`/`journeyRoute` validerar redan strukturen innan `RouteEnterMilli`; den senare har dessutom defensiv lokal skip, inte ett världsvidt fel. DB-fel i återkommande scan fortsätter vara retrybara. Ingen DB-modell eller rörelseregel ändras.

[Riktade prov](massif/arrival-fail-open.log) visar att fel i namnflushen inte stoppar rörelse och att en korrupt `march_route` i världen varken blockerar en frisk enhets ankomst eller dess claim. Den trasiga radens tidigare progress ligger kvar. [Mutation: stoppa ankomst](massif/mutation-arrival-block.log) ger `naming flush failure stopped movement`; [mutation: globalt fel för korrupt resa](massif/mutation-corrupt-global-failure.log) ger `corrupt journey blocked healthy arrival claim`. [Återställt grönt](massif/restored-arrival.log). Reproduktion: `python3 tools/place_arrival_mutation.py`. Vid misslyckad DB-flush kan rätt gå förlorad när ankomsten tar bort rutten; det är den uttryckligt godkända prioriteringen av rörelse framför namngivning.

## Vald kursiv A — nya LIVE-siktbilder

Timothy valde **A** 2026-10-12. B och frågeparameterväxeln är borttagna ur produktionskod och aktuella riggverktyg. En skarp mörk halo på en logisk pixel ur rendererns befintliga palett gör kursiven läsbar även på ljusa kalkberg; ingen platta, blur eller stadsnamnets monospace/stroke. Befintliga kollisionsmått rymmer halo och ger företräde åt städernas etikett/symbol.

[Ny process](live-labels/provenance.txt), [runtime/asset-SHA](live-labels/runtime.json), [bygge](live-labels/rig-build.log). Process **46e8f71**, migration172; renderer4820b9f7. [Firefox-/siktfakta](live-labels/facts.json) visar verklig URL utan växel, åtta etiketter och serverns live-tiers. Explicit BILD-fixtur: 50 verkliga placerade infanterienheter på framkomlig mark gav levande ögon; alla togs bort efter bilderna. Inga klient-tiers ändrades. De tidigare namngivna geografiska objekten är samma explicita namnfixtur som ursprungsbilderna; detta bevisar layout, inte en naturlig upptäckt. Varje fokushex är LIVE och minst 70% av viewportens kända mark är LIVE. FOW utanför ögonens räckvidd kvarstår.

| Objekt | Firefox1280 | Firefox390 |
| --- | --- | --- |
| Thaleia, flod | [1280](live-labels/desktop-river.png) | [390](live-labels/mobile-river.png) |
| Mount Ida, massiv | [1280](live-labels/desktop-massif.png) | [390](live-labels/mobile-massif.png) |
| Damon Sands, öken | [1280](live-labels/desktop-desert.png) | [390](live-labels/mobile-desert.png) |
| Ariadne Cedars, skog | [1280](live-labels/desktop-forest.png) | [390](live-labels/mobile-forest.png) |

[Halo bort — namngivet rött](live-labels/halo-mutation.log): `geographic label lacks its dark pixel halo`; [återställt grönt](live-labels/label-restored.log). Full JS **587/587** grön. Nya bilder väntar på Timothys BILD-dom; valet av A är redan avgjort.

## Tidigare A/B-underlag — historik före bildvalet

**A:** kursiv serif. **B:** spärrad rak serif. Båda utan platta eller stadsnamnets stroke, i befintlig rendererpalett med dämpad alpha; floden följer känd lokal fåra, övriga kinds ligger över kända områdets mitt. För liten zoom, fog/okända hexar och namnlösa objekt får ingen etikett. Faktiska textmått och konservativa roterade rektanglar reserverar stadsnamn/stadsmassa och andra ortnamn. [Mått, faktiska URL:er, fixture, hälsa](pictures/facts.json). Stadskrockprovet omfattar respektive text-/symbolruta; inte all annan kartdekor.

| Objekt | A 1280 | B 1280 | A 390 | B 390 |
| --- | --- | --- | --- | --- |
| Thaleia, flod | [A](pictures/desktop-a-river-label.png) | [B](pictures/desktop-b-river-label.png) | [A](pictures/mobile-a-river-label.png) | [B](pictures/mobile-b-river-label.png) |
| Mount Ida, massiv | [A](pictures/desktop-a-massif-label.png) | [B](pictures/desktop-b-massif-label.png) | [A](pictures/mobile-a-massif-label.png) | [B](pictures/mobile-b-massif-label.png) |
| Damon Sands, öken | [A](pictures/desktop-a-desert-label.png) | [B](pictures/desktop-b-desert-label.png) | [A](pictures/mobile-a-desert-label.png) | [B](pictures/mobile-b-desert-label.png) |
| Ariadne Cedars, skog | [A](pictures/desktop-a-forest-label.png) | [B](pictures/desktop-b-forest-label.png) | [A](pictures/mobile-a-forest-label.png) | [B](pictures/mobile-b-forest-label.png) |

Bildfixturen skriver **explicit SQL-rätt och minne** på Eirene för de fyra riktiga statiska komponenterna och använder sedan riktiga webbrutan. De 16 etikettbilderna och 8 formbilderna bevisar layout/handling, **inte** naturligt inträde (särskilt inte massiv). Naturlig flodankomst har separat bevis ovan. Denna ursprungliga prototyp hade default A utan godkännande; Timothy har nu valt A. B-bilderna behålls här endast som historiskt beslutsunderlag.

## Metodisk lärdom och avgränsning

1. Granska faktisk positionsägare före en rörelsehook; DB-koordinaten är inte det aktiva hexsteget.
2. Prioritet i en redan hämtad batch skyddar inte mot en återkommande scans catch-up-clamp; prova hela kön.
3. Namnrätten måste gå att nå där aktörens/stadens panel faktiskt ersätter terrängkortet.

Budets ruttmodell är avgränsad enligt ovan; massivets nya grannmarksregel har DB-/API-prov nedan, ingen separat naturlig live-expedition. Genomförd naturlig marsch och namngivning avser flod; desert/cedar har riktiga sparad-rutt-/geometriprov och explicit bildfixture, inte en separat naturlig live-expedition var. Ingen backfill av gamla inträden, ingen positionsrekonstruktion för förstörda aktörer. Bilderna är Firefox vid normal zoom, inte en ny full tre-motorsparitetsrunda eller fysisk mobil/Safari. Eventuell ytterligare förstörande periodisk handlers catch-up-fysik följer befintliga rörelseregler; den här slicen gör ingen ny motor för historisk rörelse.

Reproduktion och muterande verktyg finns i `tools/README.md`. Kör mutationerna före fullsuite, inte samtidigt; tillfälliga sourceändringar återställs i finally. Privata CLI-configs/tokens stannar i temporära mappar och finns inte i bevispaketet. Dockerloggarnas carriage returns/slutblanksteg normaliseras inför commit, utan att sakutfall ändras.
