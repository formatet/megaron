# Order 26 — civil hunger som övergång

## SAMMANFATTNING

Dagliga `FoodShortfall`-notiser ersätts av `CivilianStarvationStarted` och
`CivilianStarvationEnded`. Det befintliga dagliga audit-eventet behåller sin typ
och sin nyttolast. Produktion: ursprungligen `3436b498`, nu med tröghet `e31ce04b`, gren `codex/svaltnotis-overgang`, bas
`09e14572`. Ingen migration, balansändring eller ändring av historiska arkivrader.

Tillståndet ägs av slice14:s `recordTransition` och `kharis_notice_state`.
Civil hunger och garnisonshunger prioriteras och avslutas oberoende i webb/keryx.
Codex `food` beskriver de nya notiserna och återhämtningsfönstret utan att frysa
konstanten som spelarinvariant; gamla typer behåller sitt uppslag.

**Tröghet enligt orderns tillägg:** slut kräver fem sammanhängande observerade
mätta ticks (`civilianRecoveryTicks`). Ny hunger inom fönstret återställer räknaren
men behåller samma varning. Eurymedes varannanticksmönster ger nu **en start,
inget slut under 70 ticks och ett slut på tick74**, fem mätta ticks efter sista
hungern på tick69. Kod `e31ce04b`, kompletterat verkligt måltidsprov `3f89c270`.
Ingen ny mekanism: räknaren ligger i befintligt owner-scoped JSON-context, under
samma radlås, transaktion och tick-claim. Ingen migration eller matbalansändring.

## SLICE-KONTRAKT

| Fält | Kontrakt |
|---|---|
| Problem | Återkommande civila matunderskott begraver viktiga varningar. |
| Spelarsanning | Första observerade hungriga måltiden anger stad, tick/dag och saknad mängd; stadig återhämtning avslutar varningen; återfall inom fönstret fortsätter samma period. |
| Invariant | `FoodShortfall`-audit behålls per underskott; notisminne överlever retry, omstart, arkivradering och ägarbyte. |
| Scope | Befintlig starvation-observer, FoodTick-notisproducent, webb/keryx och Codex `food`. |
| Non-scope | Matbalans, måltidsordning, gamla rader, ny tabell, pagination eller nya notisstilar. |
| Acceptans | 70-ticksprov, korrekt mängd/datum, atomisk notis/övergång, oberoende hungerarter, daglig-notis-mutation röd. |
| Stopp | Ingen ändring utanför den beställda notiströgheten. |
| Kedjegrind | **Bevisar** att verklig matbrist blir synlig så spelaren kan ordna mat utan utvecklaringrepp. |

## BASLINJE

`09e14572`, färsk DB189: hela economy/kharis/keryx gröna (`baseline-go.log`),
652 JS gröna (`baseline-js.log`). De nya pipelineproven faller på gamla koden:
den dagliga hubbnotisen går fel väg och civila övergångar saknas (`contract-red.log`).
Ett första testutkast saknade testadapterns `BroadcastEvent`; det rättades före
riktiga kontraktsröda körningen. Den gamla förväntningen om en daglig hubbnotis
ersätts av ett explicit audit-och-ingen-daglig-inbox-prov.

## EXPERIMENT

| Hypotes | Ändring | Resultat | Beslut |
|---|---|---|---|
| Samma övergångsägare kan följa civila måltider | Ny condition i befintlig state; `food_unmet_amount` som growth också läser | Inga nya tabeller/importkanter; befintligt garrison-audit kvar | Behåll |
| 70 ticks behöver inte ge 55 inboxvarningar | Riktig FoodTick → observer, tre perioder; varje event körs två gånger med nya instanser | 55 audit, tre starter, tre slut, sex postcommit-leveranser | Behåll |
| Återhämtning får inte blanda armé och folk | Fisk mättar folk samtidigt som riktig garrison-witness fortsätter | Civil recovery1 / garrison recovery0; klienter prioriterar separat | Behåll |
| Arkivradering/ägarbyte får inte nollställa privat minne | Befintlig owner-scoped state och stängning med sparad kontext | Ingen repris efter delete; ny ägare får eget startbesked; gamla ägaren får endast gammalt känt namn | Behåll |
| Notisfel får inte stoppa mat eller världen | Avsiktligt CHECK-fel vid notisinsert | Matutfall kvar, nästa KharisTick köad; återställd persistence ger en enda start | Behåll |
| Varannanticks-hunger måste vara en period | Beständig återhämtningströghet; riktig FoodTick även under återhämtning | Audit35 / start1 / slut0 vid70; slut1 först74, retry/restart ingen dubblett | Behåll enligt tillägg |
| Fyra mätta ticks räcker inte vid återfall eller lucka | Hunger efter fyra mätta; oobserverad tick; fel vid femte tickens notisinsert | Räknaren återställs, luckan räknas inte, misslyckat slut rullas tillbaka; retry ger ett slut, senare hunger ny start | Behåll |

## GRINDAR

| Grind | Bevis |
|---|---|
| Kod | Full `tools/gotest.sh` mot färsk DB189 grön, inklusive world137,399s (`full-go.log`); `go vet ./...` grönt. Full körning startade på samma produktionskällträd före commit (loggens bas09e+ändringar). Extra alternansprov grönt på färsk189 efteråt. Tillägget verifieras i `inertia-full-go.log`, `inertia-vet.log`, `inertia-js.log`. Första samtidiga fullsvitsstarten avbröts när mutationsöverlapp upptäcktes; räknas inte som bevis (`inertia-full-go-discarded-overlap.log`). Slutlig fullsvit startades efter alla mutationers byte-exakta återställning. |
| Klienter | 654/654 JS; riktade Go-keryx/known-kind-prov gröna; nya typer har domän, ikon, text, dag och Codex-artikel. Små positiva matunderskott visas som `<0.1`, inte0. Historisk `FoodShortfall`-text kvar. |
| Mutation | Fem fysiska mutationer: borttagen tröghet (5→1), återställd daglig hubbnotis, avstängd civil start, återställd daglig transition, gemensam klientnyckel för folk/armé. Namngiven RED → byte-exakt restore → färsk GREEN (`mutations.json`, tillhörande loggar). Nytt kontraktsprov är också rött på föregående416800db: `inertia-contract-red.log` visar35/35/35 mot beställt35/1/0. |
| Visuell/TEXT | Befintliga notisrader, inga CSS- eller layoutändringar. BILD krävs inte enligt ordern. Textens begriplighet till vanlig playtest; Firefox provas i naturliga flödet nedan. |
| Semantisk | Samma faktiska matutfall som tillväxten använder. Notis/övergång/event skrivs i samma TX; WS först efter commit. Ägar-/inaktivitetsstängning säger "warning closed", inte att folk blivit mätta. |
| Användare | Naturligt register → CLI login/join/found → vanliga placement-endpoints → riktig hunger/recovery → CLI watch/archive och Firefox1280/390. Grönt på process `3436b498`/mig189: start tick293, slut tick297. En riktig start och ett riktigt slut i CLI-watch/API/Firefox, inga gamla FoodShortfall-inboxrader. `native-a/proof.json`, `watch.txt` och fyra skärmdumpar; 0pageerrors/overflow. Ingen SQL-skrivning eller replay. |
| Drift | Ingen merge/push/deploy; Claude integrerar. Serverprocessens `/healthz`: commit3436b498, migration189, statusok. Master har gått vidare under arbetet; Claude integrerar från bas09e14572. |

Tilläggets naturliga användarprov: `native-inertia-paced/proof.json` på process
`3f89c270`/mig189 (samma produktionskod som `e31ce04b`).
Vanlig registrering, CLI join/found, placement-API, riktig hunger och återhämtning,
CLI watch/arkiv/Codex samt Firefox1280/390: **GREEN**, hunger281 → slut288
med `fed_streak=5`; exakt en start/ett slut, riktig processomstart och återansluten
CLIwatch, inga pageerrors/overflow. End-notisens mättnadsräknare verifieras i riggen.
Första1s-riggen (`native-inertia/`) blev korrekt utan slutnotis: tickarbetaren
och makroarbetaren missade observationer, så streak stannade på1 trots mätt stad
(`context-diagnostic.csv`). Även2s-riggen (`native-inertia-two-seconds/`)
visade luckor och avbröts tidigt; räknas inte som slutbevis. Slutreceptet behåller
1s tills hunger och startar sedan om samma binär/DB till12s/tick enbart för
återhämtningen (befintlig eventarbetare har10s-polltak). Ändringen är väggtakt,
inte spelregler eller world-state; CLIwatch återansluter. Inga SQL-skrivningar.

## METODISK LÄRDOM

- Ett start/slut-kontrakt betyder inte automatiskt färre rader. Prova faktisk
  periodlängd, särskilt om fyndet beskriver varannanticksförlopp.
- Naturlig svält måste invänta hela matförrådet, inklusive det Host bar med sig.
  Första riggens120s räckte inte. API-diagnostik visade fortfarande110,4ticks mat
  vid tick97 (`native-budget-too-short/waiting-food-diagnostic.json`). Slutriggen
  härleder väntbudgeten från aktuellt förråd, konsumtion och underskott, utan att
  ändra några resurser. CLI watch väntar på riktiga händelser; ingen API-polling.

## KÄNDA AVGRÄNSNINGAR

- Första notisen anger första lyckade observationen, inte en rekonstruerad
  historisk hungerstart. Historiska rader ändras inte.
- Prioriteringen återanvänder slice14:s laddade arkivfönster (API högst100rader).
  Pagination och hämtning av äldre aktiva starter utanför fönstret ändras inte.
- Daglig audit använder FoodTicks befintliga append/claim-väg; denna slice
  ändrar inte den äldre postcommit-appendens felmodell. Nya övergångsnotiser är
  atomiska med sitt persistenta minne och isolerade från matdebitens TX.
- Saknade observationer bevisar inte en mätt tick; återhämtningen räknas om från nästa observerade mätta tick.
- En notispersistensstörning rullar tillbaka den gemensamma observationsbatchen,
  loggas och kan återobserveras; den hindrar inte nästa världstick.

## REPRODUCERA

```sh
tools/gotest.sh
node --experimental-default-type=module --test $(rg --files web/static/js | rg '\.test\.mjs$')
python3 docs/reviews/svaltnotis-overgang/mutations.py
python3 docs/reviews/svaltnotis-overgang/native.py docs/reviews/svaltnotis-overgang/native-inertia-paced
```

Mutationerna skriver temporärt till källfilen: låt dem bli färdiga före annan
kompilering eller naturlig serverkörning. Native kräver rena server/web-filer,
Docker, Go och Python/Playwright Firefox. Världens testkadens är1s/tick; reglerna
är oförändrade. Ingen SQL-skrivning eller HTTP-replay används i det naturliga provet.
