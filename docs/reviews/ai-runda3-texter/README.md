# AI-runda 3 — slice 7

Bas229c291e, `codex/ai-runda3-texter`. Klar för Claudegranskning; Claude integrerar.

## Kontrakt före kod

| Fält | Innehåll |
|---|---|
| Problem | Status, matplacering, arbetskraftsskala och CLI-kommandon ger motsägande eller ofullständiga handlingsbesked. Blockerar spelarens förståelse av brist→produktion. |
| Spelarsanning | Befolkningens mat och arméns kornuttag visas med tydliga separata betydelser; lediga matgubbar, skala, ortval och tider går att agera på. |
| Invariant | Ingen ändring av FoodTick/tillväxt/upkeep, FOW eller placeringsregler. Samma serverägda underlag i webb/CLI; befintliga argument fungerar. |
| Scope | Orderns nio punkter: status/surplus, city/placements/place/goods, founding --yes, ortalias, tom borttagning, delad hex, ETA; webb/serverunderlag/Codex där motsvarande besked finns. |
| Non-scope | Byggkö, balans, nya mekaniker och automatisk omplacering. |
| Acceptans | 1) Inget motsägande netto utan namngivna avdrag. 2) Samma matarbetskraftsbesked och begriplig skala. 3) Alias/borttagning/--yes fungerar. 4) ETA speldagar och delad hex förklaras. 5) Riktade regressioner/mutation, fullfreshGo/vet/JS och naturligt CLI/webb-flöde. |
| Stopp | Om fynd kräver ny mekanik eller ändrad kanon: avgränsa och rapportera till Claude; fortsätt övriga godkända textfixar. |
| Bevis | Baslinje från aktuell kod/rena testfixturer; fysisk mutation per ändrad logik med namngiven assertion RED och exakt återställning; ny PG per arm. Egen naturlig server/CLI/Firefox med healthz-proveniens. |

## Sammanfattning och nio fynd

Kärna `ccaf491e`, sista Go-textputs `95eeb533`, mobilförklaring `f657a1ab`, från `229c291e`. Ingen migration, ingen push/merge/deploy av Codex. **Full FÄRSK Go182 exit0**, source95eeb533/world143,227s (alla Go-bytes är oförändrade i f657a1ab); vet ren, **651JS/0skip** och naturlig rigg på sista webbversionen gröna.17 namngivna fysiska mutations-RED→exakta bytes→GREEN.

| Order | Resultat |
|---|---|
| 1. Två matnetton/Net net | Befolkningens grain+fish−måltid märks **before army**. Nytt additivt `food_net_per_tick_after_upkeep` = samma foodnet−verkligt kornunderhåll, på både province och overview via en delad helper. Keryx visar `Army eats X grain → food net Y after army`, silver separat. Webbens stad visar båda leden; översiktens kolumn heter **Food after army** och använder det nya serverfältet. |
| 2. Motstridiga matgubbetal/★/grammatik | Matens mottagarvarning med ett eget gubbetal utgår; gemensamt required/placed-råd finns kvar och använder `foodSurplus`. Det gäller bästa matplatser **för befolkningen**, därför säger rådet att kvarvarande matgubbar kan behöva arrangeras om. Produce/produces och citizen/citizens stämmer. ★ på egna bemannade matrader fungerade redan; verifierat, inte en påhittad lagning. Nytillagd spärr hindrar främmande hexgubbar från att markeras som egna flyttbara. |
| 3. Skala | Status/city/goods: **1 citizen =100people**; status Labor räknar hela citizens; goods Workers→Citizens,200/500→2/5. API/JSON:s råa personantal är oförändrade. Webbens labor-detalj och matplatsrad säger skalan; Codex citizens förklarar. |
| 4. --yes frågar ändå | Kan **inte reproduceras** i aktuell kod: `if !yes` fanns redan. Faktiskt command-test och naturlig register/join→grundning med vanligt byggt CLI visar ingen fråga; fysisk mutation som kringgår yes-skyddet gör regressionen röd. Ingen onödig ombyggnad av bekräftelseflödet. Kontrollera vilken CLI-binär playtestet kör om fyndet återkommer. |
| 5. before other use | Säger nu **if nothing else uses it**. |
| 6. --province | City/placements tar flagga och befintligt positionsargument. Status/build tar flagga eller positionsargument. Place tar `place [city] <good> <ordinal> [+n|-n]` eller flaggan före varan. Alla använder befintlig namn/UUID/FOW-resolver; dubbla väljare nekas tydligt. Flagga före varan behövs fortsatt för att en bar−1 ska behandlas som delta. |
| 7. Tom −1 | Vid noll där visar kommandot var stadens egna gubbar arbetar med varan, t.ex. `#13 fish (2 citizens)`. Gör ingen DELETE när ingen finns; hittar ingen egen bemanning säger den det. Främmande hållares globala beläggning listas inte som egen. |
| 8. Delad hex | CLI och webbkort säger **already worked by another city**; befintliga hållar-/takeable-regler oförändrade. |
| 9. Klocktid+dag | Gemensamma rese-ETA och hostens förrådstid visar dagar utan klockparen. Vid okänd kadens/oläslig ankomst säger CLI **arrival time unavailable**, inte en gissad dag eller rå tidsstämpel. Absoluta världsdagsbesked för expeditionshemkomst behålls. Händelseloggens tidsstämplar är inte res-ETA och ändras inte. |

## Baslinje, betydelse och avgränsning

Orderns Scheria-fall återges i regressionen: grain0,5 +fish5−people5 =+0,5, arméns grain2 ger **−1,5food after army**, medan det gamla grain-only-fältet är−6,5. Tidigare källkod presenterade det sistnämnda som `Net net…`; regressionen förbjuder det i utskriften. Egenskaperna bevisas med actual HTTP-kommandon och riktig DB/API, inte enbart textgrep.

Det är två olika balansbegrepp, inte två nya utfodringsregler. Tillväxten använder fortsatt befolkningens `economy.FoodNet`. Armén äter fortfarande **korn först, aldrig fisk**. CLI:s truppvarning jämför därför arméns kornbehov med kornproduktionen före folkets måltid; den gamla läsytan drog av alla människors mat som korn igen även när fisk födde dem. Ett fixtureprov med1grain+5fish/1000personer och0,5armygrain får ingen falsk truppsvältvarning vid tomt kornlager; armén får sin0,5 först och folket äter resten/fisk. Ingen tick-handler, konsumtion eller enhetsupkeep ändras.

`net_grain_per_tick_after_upkeep`, dess grain-only-semantik och befintliga recruit-kapacitetsfält finns kvar för API-kompatibilitet. Denna slice lagar de beställda status-/översiktsytorna; läsare ska använda det nya fullfood-fältet för just matbalans efter armén. Totalfoodnet ersätter inte truppernas separata krav på korn. Rålager är fortfarande lager, inte en försäkran om hur länge en oförändrad stad räcker.

Matplatsrådet är en jämförelse med optimala platser för befolkningen, ingen order att godtyckligt plocka bort tre valfria gubbar och ingen dimensionering av arméns korn. Det sägs uttryckligen. Lagerkapacitetsvarningar för andra varor finns kvar; matens separata gubbetal har tagits bort för att inte tävla med samma stads food-required/placed. Ingen automatisk omplacering införs.

## Experiment och grindar

- `TestAIRound3_FoodAfterArmyAPIParity`: ny PG16/mig182, ägd fiskstad+underhållen galley, båda riktiga HTTP-endpoints ger samma fullfood efter armén; befolkningens foodnet/tillväxtunderlag kvar+0,5. Återanvänder verkliga payer-/statusfiltren i `settlementUpkeepDrain`.
- `TestAIRound3_*` CLI: faktisk Cobra/HTTP-körning för status, tio kombinationer av flagga/position i city/placements/status/build/place, tom borttagning utan DELETE, skala200→2, --yes utan fråga, egna★/främmande★, grammatik, ETA och väntetid. Dessa klienttester använder httptest; de är inte live-spel.
- **17 fysiska mutationer** i `mutations.py`: API-armyavdrag, grain-only som fullnet, konkurrerande matplatsvarning, cityflagga, status/build/placeposition, personskala, tom-hjälp, främmande-hjälp, ETAklocka, väntetidstext, webbens två balanser, yes-skydd, främmande★, arméns prioritet. Varje giltig mutation ger namngiven assertion **RED**, exakta originalbytes återställs, ny körning **GREEN**. API-armarna har var sin färsk PG. Inget kompileringsfel räknas som mutation.
- JS:651tester/0skip gröna inklusive båda nya webbbalanserna och Codex-/dagsvakterna. Vet ren. Full färsk Go-svit på95eeb533 är grön, world143,227s; sista commitf657a1ab ändrar bara webbtextens radbrytning. Första fullsviten påccaf491e är också grön, world137,913s. Slut-Vet är ren.
- Naturlig användargrind: `native.py` startar egen PG16/Redis/server med ren miljö; register→join→byggt CLI→preview→grundning --yes→alias/stadsval→tom borttagning→riktig −1/+1→status/goods och Firefox. Ingen SQL-skrivning, inga response-overrides. Riggkonton skapas endast där; GPT-spelkonton/livevärlden berörs inte.

Första naturliga körningen visar CLI/båda webbnetton på `ccaf491e`/182 och `page_errors[]`. Körningen på95eeb533 scrollar fram den ändrade matdetaljen och visar+0,6före/−0,4efter armén. Den hittade en trång matplatsrad i390px; förklaringen flyttades till ett eget stycke i f657a1ab. Sista naturliga körningen har **healthz f657a1ab7c508b1bff974a5b2f2d0e36a18147e2/182**, page_errors[], och den texten är läst i1280/390 vid1:1. Dess slumpade kuststad har annat jordbruk/fiske och visar1före/0efter; den negativa Scheria-varianten är dessutom låst i DB-/klienttesterna. Browserbilder är funktions-/TEXT-bevis; ingen ny grafik eller BILD-gate introduceras. Privata authconfigar/binaries exporteras inte.

Apparatavvikelser: ett första test försökte spara en yes-variabel som i verkligheten är lokal (kompilationsfel, rättat). En mutationsankare för employed/100 matchade två ställen och stoppades **före mutation**; snävades till det riktiga Workers-uttrycket och fortsattes därifrån. Codexdagsvakten fångade uttrycket game days i ny spelarprosa; ändrat till days. Äldre ETA-tester som uttryckligen krävde klockparentes följer orderns nya konvention. Inget av dessa räknas som giltig mutations-RED.

## Reproduktion

```sh
python3 docs/reviews/ai-runda3-texter/mutations.py
python3 docs/reviews/ai-runda3-texter/native.py /tmp/megaron-runda3-fresh
# Välj fri GOTEST_PORT; gotest.sh kör med ren miljö och ny PG16.
tools/gotest.sh
```

Överlämnas som branch+hash till Claude för granskning/integration. Efter merge måste även **playtestets CLI-binär byggas om från den integrerade koden**; en serverdeploy kan inte byta en separat gammal CLI-binär. Ingen migration/recompute behövs för denna läs-/textslice.
