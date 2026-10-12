# Färdvägsstreck

Kartan visar nu en alltid synlig återstående färdväg för egna arméer/Host, skepp, bud och karavaner. Avtalade leveranser till dig får också framåtstreck; övriga främmande aktörer får endast en aktuell kompasspil. All ritning klipps till LIVE-sikt, även enskilda halo-/skuggpixlar.

Gren `codex/fardvag` från **746398ed**, worktree `/home/tk/wt/codex-fardvag`. Inventeringen nedan committades **07213863 före produktionskod**. Server/CLI **f7a7201d**, avtalad mottagare **2b3074ab**, webb/Codex **cdfe6abb**. Claude granskar och integrerar. Timothy är bortrest: hans BILD-grind undantogs uttryckligen i Claudes meddelande **2026-10-12 07:54**; Claude granskar bilderna själv. Inga merge/push/deploy av Codex.

Färsk full Go, vet och **598 JS-prov** är gröna. **Åtta fysiska mutationer** gav namngivet rött och återställdes till grönt. Firefoxbilder för nio fall × 1280/390 samt en vanlig spelarorder till faktisk ankomst finns nedan. [Samlat process- och testbevis](verification.json).

## Ursprungligt kontrakt före implementation (mottagarundantaget rättades 07:46–07:54)

| Fält | Kontrakt |
| --- | --- |
| Problem | Kartan visar gående aktörer utan en alltid synlig återstående färdväg eller främmandes riktningspil. |
| Spelarsanning | Mina aktörer visar vägen framåt från aktuell position; främmande visar endast aktuell riktning. |
| Invariant | Inget bakom aktören, ingen främmande rutt/mål/ETA, inga streck/pilar utanför LIVE-sikt. |
| Scope | Serverns befintliga kartprojektioner där inventeringen visar saknade fält; delad webbrendering, keryx sightings, Codex, prov/rigg. |
| Non-scope | Rörelsebalans/pathfind, nya diplomati-/krigsregler, ny budruttlagring, kingdom/oikos. |
| Acceptans | Progress0/mitt/1 och LIVE-klippning; säker foreignpayload+mutation; fyra ytor; färsk Go om server rörs/JS/vet; Firefox1280+390 med fyra egna slag och främmande armé/skepp. |
| Stopvillkor | Ny relation eller ändrad informationsrätt behöver kanon; recipient-transporter särskilt avstäms före DTO-ändring. |
| Bevisplan | Baslinje, verkliga API-payloads, rutt-/privacyprov och fysisk mutation, aktuellt processbygge, BILD1280+390. |

Kedjegrind: bevisar kartans geografiska läsning och orderriktning, utan ny brons-/elitmekanik.

## Inventering före kod

| Slag/relation | Befintlig payload och positionsägare | Lucka/minsta ändring |
| --- | --- | --- |
| Egen armé/Host | `/units`, owner-only `ListUnits` (world.go2016). `attachUnitPaths`2108 skickar frusen `LoadActiveRoute`-path; `unitSummary`2181 bär ISOtider och arrival_tick/duration_ticks. `current_q/r`2726 kommer från sparad rutt. | Step costs saknas för vägvisning i samma terrängtakt; lämna NULL/legacy ärligt utan påhittade kostnader. Kartans unitPixelNow interpolerar idag jämnt. |
| Eget skepp | Samma `/units`; category naval, sparad marschrutt. Fraktens sjöresa bor i transports och `/trades`. | Samma routefält + sjöfraktens sparade transportväg; inget nytt dispatch-/rörelsesubstrat. |
| Egen budbärare | `/messengers` (world.go1405): own, endpoints/tider; aktuell fysisk passageleg1592–1653, carrier-id vid aboard. Ingen lagrad hexrutt. | Exponera den befintliga modellens A*-väg för aktiv markleg; waiting/sealed ger ingen framåtväg, aboard följer faktisk carriers sparade väg. Detta är inte ny budruttlagring. |
| Egen karavan/sjöfrakt | `/trades`1223: transports.journey/departedTick/dueTick1261, SavedPosition1334 äger current_q/r. DTO1279 saknar path/costs/category/ship-id. | Exponera validerad frusen TradeJourney endast för ägaren. Ursprung/mål i sparad sjöväg kan vara vattenankare en hex från stadskoordinaten. |
| Främmande armé/Host | `/foreign-units` dto28–69: current q/r, owner/typ/stance, heading; FOW191–198. ReadMarch180–184 äger kompass NU. Ingen path/target/trueETA. toward är uppskattning till betraktarens egen stad, inte faktisk främmande ETA. | Rita pil från befintlig heading. Rätta stale kommentar om full rutt i foreign_units.go90. Ingen ny synlighetsregel. |
| Främmande skepp | Samma säkra `/foreign-units`, category naval. | Samma pil, aldrig path; ingen förändring av främmande armé/skepps positionsmodell. |
| Främmande budbärare | Skickas av `/messengers`; aktuell-position-FOW1668–1683, aboard/sealed döljs1662. DTO1449 skickar trots det origin/dest och ISOtider; heading saknas. | Säker foreignprojektion current q/r + ReadMarch-heading under samma grind. Inga egna leg-/carrier-/order-/endpoint-/tidfält på foreign. |
| Främmande karavan/sjöfrakt | Skickas av `/trades`, live-current-FOW1363–1383 för utomstående. DTO läcker endpoints/ISO;1390 tömmer endast cargo+tickETA. Mottagare role=recipient bypassar redan FOW och får känd cargo/ETA (map_trades_mine_test255/385). | Säker foreignprojektion current q/r + kompass NU, inga rutt/endpoint/tidfält. Mottagarundantaget avstäms med Claude före ändring; cargo/roll och ordinarie handelsdetaljer behöver inte försvinna. |

Keryx `sightings`: idag bara `/foreign-units` + provinser (cmd_sightings.go104/122); armé/skepps-heading skrivs redan170. Behöver även bud-/karavanernas säkra kartprojektion om servern lägger till riktning. Ingen ny kommandohandling.

Relationer i befintlig MVP: egen vs främmande neutral/okänd. `FOREIGN_ACCENT` är befintlig violettgrå, vald tidigare för diskriminerbarhet mot sprites/terräng; inga förklarade krig/alliansenum finns på dessa kartpayloads. Egen färg + framåtstreck, främmande färg + riktningspil ger redundant form; slag bärs av olika streckmönster. Två BILD-relationer behöver inte uppfunnen diplomati. Avgränsningen är skickad till Claude.

## Baslinje

[JS på oförändrad746398ed](baseline-js.log). Server-/process-/bildbaslinje tas före ändring av respektive yta. Befintlig isolerad rigg ska styrkas med sin egen healthz/provenance; sourcecommit och gammal process får inte sammanblandas.

## Experiment och grindar

### Slutligt kontrakt och informationsrätt

- Egen armé/Host, skepp, karavan och aktiv budleg: framåtstreck från aktuell position, aldrig en passerad sträcka. Strecket krymper och försvinner när målhexen nåtts; serverns ankomsthandler kan verkställas senare.
- **Avtalad mottagen transport:** `role=recipient` och kind **`trade` eller `trade_return`** får `route_known=true`, frusen `path` och `step_costs`. De uppstår ur accepterad bilateral handel (`messenger.go` accepteringsgrind och `economy/trade.go` returben). Samma framåtgeometri som egna, men befintlig främmandefärg.
- **Inte avtalade för mottagaren:** `gift`, `transfer`, `standing_order_out`, `standing_order_return`, `plunder`, `ship_return`, `damaged_return`. De får pil; okända framtida kinds faller också till pil. Ägaren ser sin egen väg för alla dessa kinds. Stående orders är ägarens order, inte ett avtal med mottagaren.
- **Bud från annan Wanax till mig:** pil, även när jag som mottagare har ordinarie korrespondensdetaljer. Budets väntande/förseglade ben har ingen framåtväg; eget aboard-bud följer den faktiska bärarens sparade rutt.
- Avsändare/mottagare behåller befintliga cargo-/ETA-/korrespondensdetaljer. **Utomstående får dessa inte från servern:** deras kartprojektion innehåller aktuell hex och heading, utan faktisk path, mål, endpoints eller restider. Befintliga server-FOW-grindar är intakta; webben kräver dessutom LIVE för varje ritad pixel.
- Relationer är befintlig egen/främmande (`FOREIGN_ACCENT`); ingen ny diplomati. Slag bärs av fyra mönster: armé heldraget, skepp streckat, bud prickat, karavan streck/prick. Gemensam heltalspixelritning, högst fyra färger, inga mjuka kanter.

### Position, tid och fyra ytor

`routes.js` äger samma position och progress för sprite, hover och streck. Frusna marsch-/transportkostnader följer **movement.PositionAt valA**: tiden tillbringas inne i den inträdda hexen, så första inträdda hexen äger aktören redan vid avgång. Den gamla jämna interpolationen låg en hex efter i dessa resor. Vanliga bud följer befintlig `ReadMarch` och aktuell A*-leg; ingen ny budruttlagring eller pathfindändring.

`GET /worlds/{id}` skickar verklig `tick_anchor_at`. Webbens nedre `world_time.js` delar ankaret mellan karta och tidsyta. Rörelsefasen följer tick-ankarens milli-clamp; en pausad worker ger inte fantomförflyttning förbi dess senaste tick. Befintliga uppskattningar av främmande armés färd till **min** stad är kvar; de är inte dess verkliga mål/ETA.

Temenos exponerar säkra projektioner, Keryx `sightings` skriver även bud-/karavanheading, webben ritar dem och Codex [Sight](../../../web/static/codex/sight.md) beskriver rättigheten. [Verklig Keryx-utskrift med armé, skepp, inkommande bud, avtal och gåva](pictures/sightings.txt).

### Kodgrind och mutation

| Prov | Bevis |
| --- | --- |
| Baslinje på 746398ed | [587 JS](baseline-js.log), [färska API/CLI-prov](api-baseline.log) |
| Full Go, färsk separat databas till migration 172 | [full-go.log](full-go.log) |
| Go vet | [vet.log](vet.log), exit 0 (tom logg) |
| Full JS | [598/598](full-js.log) |
| API-mutation: främmande path, fel returben | [namngivet rött och återställt grönt](api-mutations.log) |
| API-mutation: avtalsundantag bort, gåva felaktigt avtalad | [namngivet rött och återställt grönt](agreement-api-mutations.log) |
| Webbmutation: gammal sträcka, LIVE-grind, foreign path-läsning, mottagarundantag | [fyra namngivna fel och byte-återställt grönt](client-mutations.log) |

Geometriproven täcker progress 0/mitt/1, viktade inträdesgränser, gemensam funktion för fyra slag, okänd/remembered mark samt pixelklippning av pilhalo. Getterfällor gör varje försök att läsa främmande path/mål/tider rött. Produktionsscenens prov täcker avtalad mottagare, gåva och inkommande bud. Fullsviterna kördes efter återställda mutationer.

### Process och metod

Bildprocessen är **cdfe6ab / migration 172**, inte baslinjen. Bygg: CGO=0 lokal Go med cachad acceptansruntime, aktuella migrationer och worktree-webb monterad. [Runtime](rig/runtime.json), [provenance](rig/provenance.txt), [bygglogg](rig-build.log). Serverade `map.js`, `routes.js` och `world_time.js` har bytejämförts med källan; SHA256 finns i [verification.json](verification.json). Slutcommit efter cdfe6abb innehåller endast testassert, verktyg och bevis/docs; produktionskoden är densamma.

BILD-fixturen ligger i en **egen arkiverad värld 1588ebaf**, med fryst tick 128, giltiga spearman/galley och validerade TradeJourney-kostnader/durationer. Normal LIVE-sikt kommer från två egna städer och de marscherande egna armé-/skeppsaktörerna. Inga browserpayloads eller FOW-tiers ersätts. Art-gångfasen låses för jämförbara bilder. Detta är en uttrycklig SQL-layoutfixtur, inte en naturlig dispatchkedja; den ordinarie aktiva acceptansvärlden bevaras. [Scenario](scenario.json), [aktuella verkliga API-payloads](after/payloads.json), [bildfakta](pictures/facts.json): varje fokusaktör är LIVE, rätt route/arrow-primitiv finns, inga JS-fel.

Förebilderna i [baseline-ui](baseline-ui/facts.json) använder **riktiga JS-moduler ur 746398ed**, levererade genom browserinterception, mot **samma aktuella kompatibla cdfe6ab-backend**. De visar den äldre webben utan streck/pilar; de är inte bilder av en gammal serverprocess. [Tidigare processmetadata](baseline/process.json) är separat; den första scratch-fixturen var inte slutligt giltig och dess payload används inte som acceptansbevis.

### Riktig spelarorder, utan SQL-skrivningar

På den ursprungliga aktiva världen **3b5fb9cd** loggade **ParityNomad** in normalt. **Nomadic Host of Eumedes** fick en vanlig API-marsch från **(41,74) till (44,71)**, duration **6 ticks**. Verklig `/units`-progress och webben följdes till faktisk ankomst: strecket syntes under färden och var borta efter ankomst. Ingen SQL-mutation eller browserdataersättning. [Scenario och tidsföljd](live/scenario.json), [enhet och ritprimitiv](live/moving-unit.json), [Firefox i färd](live/desktop-natural.png), [Keryx i den aktiva världen](live/sightings.txt), [körlogg](live.log).

### Firefox BILD 1:1

| Fall | 1280 × 900 | 390 × 844 |
| --- | --- | --- |
| Egen armé | [bild](pictures/desktop-army.png) | [bild](pictures/mobile-army.png) |
| Eget skepp | [bild](pictures/desktop-ship.png) | [bild](pictures/mobile-ship.png) |
| Egen karavan | [bild](pictures/desktop-caravan.png) | [bild](pictures/mobile-caravan.png) |
| Eget bud | [bild](pictures/desktop-runner.png) | [bild](pictures/mobile-runner.png) |
| Främmande armé | [bild](pictures/desktop-foreign-army.png) | [bild](pictures/mobile-foreign-army.png) |
| Främmande skepp | [bild](pictures/desktop-foreign-ship.png) | [bild](pictures/mobile-foreign-ship.png) |
| Avtalad leverans till mig | [bild](pictures/desktop-agreed.png) | [bild](pictures/mobile-agreed.png) |
| Gåva till mig | [bild](pictures/desktop-gift.png) | [bild](pictures/mobile-gift.png) |
| Bud till mig | [bild](pictures/desktop-incoming-runner.png) | [bild](pictures/mobile-incoming-runner.png) |

Förebilder har samma filnamn i `baseline-ui/`. Visuell bedömning: tunna blå egna linjer och neutralviolett mottagarväg syns mot LIVE-mark; kompasspilar står vid främmande sprites. Ingen linje bakom egna aktörer eller på grå remembered mark. Mobilens knapprader klipper inte fokusaktörerna. Det här är Firefox-BILD, inte full tre-engine-paritet. **Claude gör slutlig bild-/kärngranskning före integration.**

### Reproduktion och lärdom

Verktyg finns indexerade i [tools/README](../../../tools/README.md). Bygg riggen med separat `PLACE_PROOF_OUTPUT`, kör fixture create endast om en ny uttrycklig bildvärld behövs, capture after och pictures. `--baseline-ui` använder basens riktiga moduler med aktuella API:er. Kör mutationer sekventiellt före fullsviter.

1. Exponerad karta måste auditeras på servern; att låta webben ignorera privata fält stänger inte läckan.
2. Sprite och linje behöver rörelseägarens inträdessemantik och verklig tickfas, annars börjar vägen på fel hex.
3. En frozen layoutfixtur bevisar läsbarhet och relationer; normal dispatch till ankomst behövs separat för användargrinden.

**HASH + STOPP:** byggt och bevispaketerat på samma gren. Ingen nästa slice påbörjas utan separat START.
