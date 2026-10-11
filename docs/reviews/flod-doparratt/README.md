# Ortnamn — generellt doparrättssubstrat

## Sammanfattning

Timothy ville att spelare skulle få **döpa** landet; den gamla flodlistan gav färdiga namn. Gren `codex/flod-doparratt` ersätter listan med `world.Places`, en `place_names`-tabell, ett verb `name_place` och en notiskind `PlaceFirstReached`. Ursprung c71bf82f, rebasad på **origin/master 0db368c5**, inklusive bb49ef0d. Go-produktionscommit **a14f611a**, sista frontend/Codex-commit **17eb798d**. Inte mergat/pushat/deployat av Codex.

**HOLD för kanon/BILD:** bergshexar är oframkomliga för alla nuvarande förflyttare. Därför kan massif inte vinna en naturlig rätt genom bokstavligt inträde. Fråga skickad till Claude/Timothy: fysisk ankomst till mark intill massivet, eller avvakta massivets rätt tills separat rörelseregel finns. Ingen rörelseregel ändras utan svaret. Timothy ska även välja etikett **A eller B**. Övrigt är implementerat och provpaketet nedan är konkret för granskning; denna begränsning är inte dold som färdig massiffunktion.

Riggprovenance: [processraden](provenance.txt), **a14f611 · migration 172/172 · värld 3b5fb9cd · tick 2810 · TICK_SECONDS=6 · 2026-10-12 00:40**. Världen fortsätter ticka. [Runtime/binary-SHA](runtime.json), [aktuella branch-assets](served-assets-final.json) och [bygget](rig-build-final.log). Lokal CGO=0-server på cachad isolerad acceptansruntime med aktuella migrationer/branch-assets, samma dokumenterade DNS-fallback som tidigare slicer; inget påstående om ett rent Dockerbygge. Endast `megaron-acc`/8097 används.

## Slice-kontrakt

| Fält | Kontrakt |
| --- | --- |
| Problem | Färdiga namn undanröjer spelarens doparrätt. |
| Spelarsanning | Första egna fysiska inträdet ger en bestående engångsrätt, synlig och användbar i webb/keryx. |
| Invariant | Synlighet ger ingen rätt; kind+ankare är stabilt; okända hexar läcker inget namn eller can_name. |
| Scope | Geografi, additiv migration 172, sparad-rutt-progress, generiskt API, CLI/webb/Codex/notis/Umami, två BILD-varianter. |
| Non-scope | Stormnamn, terränggenerering, rörelsebalans, gamla rörelsers backfill, budets nya ruttmodell. |
| Acceptans | Första inträde/omkörning; 403/409/400/unikhet/admin; FOW; fyra ytor; färsk helsvit/vet, fysiska mutationer, faktisk namnhandling och Firefox 1280+390. |
| Stopvillkor | Kanonkonflikt om fysisk nåbarhet lyfts till Timothy; inga bergs-/synlighetsregler smygs in. |
| Bevisplan | Baslinje från faktisk process, persistenta lifecycle-/köprov, riktiga namnhandlingar, två bildvarianter. |

Kedjegrind: bevisar geografi-ytans handling och återkoppling; ändrar inte brons-/elitregler.

## Geografi, ägare och livscykel

| Kind | Statisk komponent | Minsta storlek |
| --- | --- | --- |
| river | river + river_ford; anslutna deltaöar ärver första kanoniska kedjan | Ingen |
| massif | mountain_limestone + mountain_red tillsammans | 5 hexar; fysisk rätt väntar kanonval |
| desert | semi_desert | 5 hexar |
| forest | Endast forest_cedar; aldrig forest_olive_grove | 5 hexar |

Ankare = minsta `(q,r)` i grundkomponenten. Samma fulla statiska geografi används av server och rörelse; klienten skapar inga komponenter ur sin partiella FOW. Namn är skiftlägesokänsligt unika **över alla kinds i världen**. Namnreglerna följer stormarnas 2–30 tecken/filter/normalisering. Bara claimed_by får döpa en gång; admin kan sätta/byta/rensa med befintlig X-Admin-Key. named_tick bevaras efter rensning. Admin kan moderera en giltig statisk plats före spelaranspråket; ett senare inträde lovar då inte en redan förbrukad namnhandling.

Rörelseaudit hittade ingen gemensam DB-skrivning för varje hex: sparade rutter interpoleras, q/r skrivs vid ankomst/avbrott. **Claude beslutade 2026-10-12 00:12** att spegla stormscanens sparad-rutt/progress-mönster. En central `recordPlaceEntriesTx` i combat sorterar faktiska `RouteEnterMilli`-inträden över alla aktörer, låser resa/progress och skriver anspråk, händelse, notis och det verkligen besökta hexminnet atomiskt. Ursprunghexen ger ingen ny rätt. Lika inträdestid bryts deterministiskt med resnyckeln. Progress har last_tick och last_milli för recall mitt i ett tick.

Täckt: **arméer, skepp och värden** med giltig aktiv sparad units.march_route; **landkaravaner och fraktskepp** med sparad transports.journey, oavsett transportärende. Fraktskeppets riktiga unit-id och kanoniska visningsnamn bevaras i notisen; karavaner använder transport-id. **Budbärare undantas uttryckligen enligt Claude:** ingen sparad hexrutt; de får inte en påhittad rutt eller synlighetsrätt. Gamla/ogiltiga NULL-rutter, orörliga garrison/positioned och förstörda aktörer räknas inte som nya inträden.

Återkommande PlaceReachScan ligger vid prioritet 7, efter stormförluster och före ankomst. Bara prioritet räckte inte vid catch-up: EnqueueTickRecurring hoppar till aktuell världstick+1, medan en äldre ankomst redan kan ligga i samma hämtade batch. **BeforeArrival** flushar därför samma centrala TX före Unit/Transport/Logistics/Trade/Gift/TradeReturn/SentryReturn/PickupTimeout. Fel stoppar den ankomsten så att rutten finns för retry. Ingen ändring av events.Worker eller andra scans. Recall/redirect flushar gamla faktiskt inträdda hexar i samma ruttbytes-TX före bytet, med gemensam världs-/radlåsordning.

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

Kod: **586 JS-prov gröna**, färsk full Go och vet gröna. [full färsk Go](full-go-final.log), [full JS](full-js-final.log), [vet](vet-final.log), Python compile och diff --check. Riktade [API/FOW](final-focused.log), [rörelse/prioritet/G1](movement-focused.log), [verklig kö/ankomst/fraktskepp](arrival-focused.log), [Host/City-räckbarhet](panel-reachability.log). G1 placerar combat → world uttryckligen; stormarna är oförändrade.

Fysiska mutationer med återställd kod grön:

- [claimed_by IS NULL bort](mutation-first-owner.log): `earliest physical entry did not retain first owner`.
- [Behörighetsgrind bort](mutation-api-right.log): `nonclaimant naming ... want403`.
- [Centralt rörelseanspråk bort](mutation-movement-hook.log): `physical movement reach hook missing`.
- [Ankomstförutsättning bort](mutation-arrival-prerequisite.log): `catch-up arrival erased last reached hex`.
- [Webbrätt och läsbar CLI-notis](client-mutations.log), med separata återställda gröna loggar.

Användare/semantik: ParityNomads **vanliga API-marsch** från `(43,73)` till deltat `(41,74)`, ticks **2623→2626**, gav rätt till river **41,75**. Ingen SQL-ändring av den aktören, rutten eller rätten. [Sparad faktisk rutt](live-host-route.json), [anspråk/namn](live-host-claim.json), [rätt/notis efter ankomst](live-host-reached.json). Sedan återupptogs just den verkliga rätten i webb/keryx-provet: [scenario](live/scenario.json), [CLI före](live/places-before.txt), [efter](live/places-after.txt), [403/400](live/refusals.json), [andra försök 409](live/second-name-409.txt). Namnet **Doria** skrevs i riktiga Firefox-rutan och visas på kartan. Två nya normalregistrerade konton provades först men hade ingen nära känd namnlös komponent; ingen geografi/rätt injicerades för att göra dem gröna.

BILD: [naturlig Host-form 1280](live/desktop-natural-form.png), [390](live/mobile-natural-form.png), [döpt karta 390](live/mobile-natural-named-map.png). Hostens titel behåller aktören; namnet visas som egen geografiinformation när aktörens/stadens panel tar över. Terräng-/ruralkortets titel visar namnet utan upprepning på TERRAIN-raden.

## Två kartetikettvarianter — Timothy väljer

**A:** kursiv serif. **B:** spärrad rak serif. Båda utan platta eller stadsnamnets stroke, i befintlig rendererpalett med dämpad alpha; floden följer känd lokal fåra, övriga kinds ligger över kända områdets mitt. För liten zoom, fog/okända hexar och namnlösa objekt får ingen etikett. Faktiska textmått och konservativa roterade rektanglar reserverar stadsnamn/stadsmassa och andra ortnamn. [Mått, faktiska URL:er, fixture, hälsa](pictures/facts.json). Stadskrockprovet omfattar respektive text-/symbolruta; inte all annan kartdekor.

| Objekt | A 1280 | B 1280 | A 390 | B 390 |
| --- | --- | --- | --- | --- |
| Thaleia, flod | [A](pictures/desktop-a-river-label.png) | [B](pictures/desktop-b-river-label.png) | [A](pictures/mobile-a-river-label.png) | [B](pictures/mobile-b-river-label.png) |
| Mount Ida, massiv | [A](pictures/desktop-a-massif-label.png) | [B](pictures/desktop-b-massif-label.png) | [A](pictures/mobile-a-massif-label.png) | [B](pictures/mobile-b-massif-label.png) |
| Damon Sands, öken | [A](pictures/desktop-a-desert-label.png) | [B](pictures/desktop-b-desert-label.png) | [A](pictures/mobile-a-desert-label.png) | [B](pictures/mobile-b-desert-label.png) |
| Ariadne Cedars, skog | [A](pictures/desktop-a-forest-label.png) | [B](pictures/desktop-b-forest-label.png) | [A](pictures/mobile-a-forest-label.png) | [B](pictures/mobile-b-forest-label.png) |

Bildfixturen skriver **explicit SQL-rätt och minne** på Eirene för de fyra riktiga statiska komponenterna och använder sedan riktiga webbrutan. De 16 etikettbilderna och 8 formbilderna bevisar layout/handling, **inte** naturligt inträde (särskilt inte massiv). Naturlig flodankomst har separat bevis ovan. Prototypens default A är inte Timothys godkännande.

## Metodisk lärdom och avgränsning

1. Granska faktisk positionsägare före en rörelsehook; DB-koordinaten är inte det aktiva hexsteget.
2. Prioritet i en redan hämtad batch skyddar inte mot en återkommande scans catch-up-clamp; prova hela kön.
3. Namnrätten måste gå att nå där aktörens/stadens panel faktiskt ersätter terrängkortet.

Budets ruttmodell och massivets kanonval är avgränsade enligt ovan. Genomförd naturlig marsch och namngivning avser flod; desert/cedar har riktiga sparad-rutt-/geometriprov och explicit bildfixture, inte en separat naturlig live-expedition var. Ingen backfill av gamla inträden, ingen positionsrekonstruktion för förstörda aktörer. Bilderna är Firefox vid normal zoom, inte en ny full tre-motorsparitetsrunda eller fysisk mobil/Safari. Eventuell ytterligare förstörande periodisk handlers catch-up-fysik följer befintliga rörelseregler; den här slicen gör ingen ny motor för historisk rörelse.

Reproduktion och muterande verktyg finns i `tools/README.md`. Kör mutationerna före fullsuite, inte samtidigt; tillfälliga sourceändringar återställs i finally. Privata CLI-configs/tokens stannar i temporära mappar och finns inte i bevispaketet. Dockerloggarnas carriage returns/slutblanksteg normaliseras inför commit, utan att sakutfall ändras.
