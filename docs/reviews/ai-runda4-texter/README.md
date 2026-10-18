# AI-runda 4 — slice 8

Bas392356a1, `codex/ai-runda4-texter`. Claude granskar/integrerar; ingen push/deploy av Codex.

## Kontrakt före kod

| Fält | Innehåll |
|---|---|
| Problem | Matens needed/move-råd, personantal, små netton, staff/hex-adresser och byggkön ger sju svårtolkade besked. |
| Spelarsanning | Food-required stämmer med verklig produktion/måltid; inget ospecificerat flyttförslag; korrekt tecken/antal; staff/build/place/queue går att handla utifrån. Blockerar kedjegrindens brist→produktion. |
| Invariant | Samma ekonomikälla och fysisk placering; ingen ny balans/byggkö eller FOW-läcka. Om needed följer en faktiskt producerande restterm ska den diagnostiseras öppet före eventuell mekanikändring. |
| Scope | Orderns sju fynd i economy/API/CLI/webb/Codex där respektive yta finns. |
| Non-scope | Nya mekaniker, köexpansion och automatisk arbetskraftsflytt. |
| Acceptans | 1) Scheria1042 i riktig DB, needed verifierad mot recompute. 2) inga oprecisa flyttlöften/fel plural. 3) +0,1 visas positivt. 4) hexstaff/ordinaler/fast kö får handlingsbara besked. 5) FullfreshGo/vet/JS, namngivna mutationer och naturlig CLI/webb. |
| Stopp | En verklig modell-/balanskonflikt redovisas till Claude innan spaken ändras; självständiga textfixar fortsätter. |
| Bevis | Kod/livedata read-only→Scheriafixture, baseline före implementation, fysisk mutation→assertionRED→exakta bytes→freshGREEN, egen naturlig PG/Redis/server med healthz. |

## Resultat per fynd

| Fynd | Utfall |
|---|---|
| 1. Needed=1 vid1042 invånare | Korrekt mot nuvarande motor. Scheria har inget farmbidrag: nearjord0.50 +42 restpersoners3.78 =4.28 automatiskt korn. En placerad fiskarbetare ger1.00; tillsammans5.28 mot behov5.21. Noll ger underskott−0.93. Vi visar det verkliga automatiska bidraget bredvid needed i API/webb/CLI och förklarar i Codex. Claude godkände visningslösningen; remainderformeln ändras **inte** här. |
| 2. Could-move | Ospecificerade flyttförslag och★ på matplatser borttagna. Minimum på optimala platser identifierar inte vilka befintliga arbetare som kan flyttas. Needed och faktiskt placed visas neutralt. |
| 3. +0.1/about even | Felet kunde inte reproduceras: befintlig formatterare använder strikt abs<0.05. Regression pinnar±0.05 och±0.1 samt verklig armyFoodLine. Ingen påstådd motorfix. |
| 4. Singular/plural | Gemensam citizenCount för berörda antal i city/status/idle/place/staff/allocate/goods/notiser; rekryterad råbefolkning kallas people. |
| 5. Staff lumbermill | Hexbyggnader hänvisas till konkret place-kommando med rätt vara, hexnummer och delta. Timber/cedar är separata riktiga roller; vald koloni bevaras i kommandot. Staff gör ingen flytt själv. |
| 6. --hex | Build/place accepterar6, #6 ochq,r. Ordinal→koordinat och koordinat→placeringsordinal läses från serverns kända meny; inga osedda hexar konstrueras. Place stödjer även positionellt#6. Flaggor före varan vid negativa delta, som befintlig parser kräver. |
| 7. Byggkö | Listing och serveravslag förklarar fast gräns2, att invånare/byggnader inte utökar den och att byggen behöver slutföras/avbrytas. Webbens uppdaterade bygglista och Codex använder samma regel. |

## Ägare och begränsning

`economy.AutomaticFoodForPopulation` äger den **oförändrade** nearjord/remainderberäkningen för recompute,
food-required, startplacering och grundningsprognos. `automatic_food` är ett additivt API-objekt med
nearjord, antal restpersoner och deras kornbidrag. Bara ägarens befintliga stadssvar får fältet.
Ingen ny balans, remainderregel, migration, tickregel, kökapacitet eller FOW-information.
Lager och Sitos påverkar inte food-required; båda prövas med stora reserver i DB-provet.
Framtida beslut om remainder hör till slice9. Den här grenen ändrar ingen GPT-spelares livekonto;
de får inte grunda innan slice10 är live enligt Claudes köorder.

## Bevis och ärlighet

`baseline-food.log` är provet mot ursprunglig beräkning före extraktionen; `food.log` visar samma
numeriska utfall efteråt. `api-food.log` prövar riktiga autentiserade HTTP-handlers mot färsk PG16,
båda stadssvarens automatiska bidrag och fullt byggköavslag. Fixtureproven skriver avsiktligt1042
invånare och Scherias terräng; de är inte ett naturligt spelprov. Första API-körningens oanvända
import rättades före grönt prov. Äldre `*-first/second/third.log` är diagnostik, inte slutresultat.

`mutations.py` ändrar fysisk produktionskod, kräver namngiven assertionRED (inte byggfel), återställer
exakta bytes och kör GREEN. Varje DB-arm får ny tom PG16 med migration182. Två första försöken för
netto muterade andra formatterare och gav grönt; de räknas inte som RED. Slutligt prov muterar den
formatterare som faktiskt äger `formatNetPerTick`.

`native.py` använder egen slumpports-PG16/Redis/server och nya lokala konton, vanlig registrering,
join, grundning, placering, CLI och Firefox1280/390. Inga SQL-skrivningar, svarsoverrides eller
GPT/livekonton. Exakt1042-gränsen bevisas av DB-provet, inte av slumpvärldens naturliga städer.
Privata tokenfiler och binärer sparas inte i git. TEXT prövas vidare i speltest; ingen ny grafik/BILD.

Slutliga verifieringsresultat finns nedan.

Naturligt slutprov: processen svarade healthz med commit`3f5205d0` och migration182.
Två vanliga grundningar, kust(43,40) och inland(13,30), följdes av faktisk borttagning via
`place --hex '#N'` och återplacering. Inlandsstadens farm hänvisade till `keryx place grain 1 +1`.
CLI city/status hade Automatic grain och inget could-move; build --queue visade fast gräns.
Webbens nya förklaring lästes i riktiga stadssvar och inspekterades1:1 i Firefox1280×900 och390×844;
texten radbryts i båda vyerna. Noll pageerrors. `native-final/evidence.json` innehåller proveniens.
Bygg-/placeringsselektorernas samtliga6/#6/q,r-varianter och FOW-avslag provas dessutom som
riktiga Cobra-kommandon mot kontrollerad HTTP, separat från det naturliga spelprovet.

## Slutkontroller

- FullfreshGo182: **GREEN**, alla paket (`full-go.log`, world152.138s), `-count=1 -p1`, egen tom PG16 och rent HOME/PATH-env.
- `go vet ./...`: **GREEN**, `vet.log` tomt och exit0.
- Webb: **652/652 GREEN, 0skip**, `js-final.log`.
- **13** fysiska kompilerande mutationer med namngiven assertionRED → exakta bytes → GREEN,
  inklusive två autentiserade API-ytor, Scheria1042, flyttråd, netto/singular, staff/hex, byggkö och webb.
- Naturligt CLI/Firefox-prov: **GREEN**, health3f5205d0/182, två grundningar, noll pageerrors.

Slutlig produktionskod och tester är exakt`3f5205d0` (kärna9be3b86e + API-städning3f5205d0).
Senare commit innehåller bara rapport, reproduktionsskript och bevis. Trädet lämnas till Claude
för egen granskning/mutation/integration. **Hash + STOPP; ingen merge, push eller deploy av Codex.**
