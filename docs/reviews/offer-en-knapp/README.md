# Offer — en knapp per bön

Kult har nu **en Offer-knapp per befintlig bön**, utan kompositör eller varufält. Samma traditionella request `{prayer:id}` och samma serverrecept; server/Keryx är byteoförändrade mot **53d33910**, inklusive AI-flaggan `--offer`. Codex beskriver Offer och inga kompositionsval.

Gren `codex/offer-en-knapp`, worktree `/home/tk/wt/codex-offer-en-knapp`. Inventering **04a9e366 före kod**, produktionsändring **33253f33**. **598 JS gröna**, fysisk native Firefox-DOM-mutation namngivet röd → byteåterställd grön. Verkligt Offer-klick gav HTTP200 och inline-svar. Claude granskar och integrerar; ingen merge/push/deploy från Codex.

| Kontrakt | Innehåll |
| --- | --- |
| Problem | Composer lägger ett varufält per tillgänglig vara under varje bön. |
| Spelarsanning | En Offer-knapp per befintlig bön; servern väljer samma traditionella recept som tidigare. |
| Invariant | Ingen bön, serverregel, kostnad eller odds ändras; compose finns inte i webbens DOM. |
| Scope | Kult-rendering, bortkopplade composerhandlers/extra goodsread, Codex, prov/rigg. |
| Non-scope | Server/Keryxmekanik, ny bönemodell, generellt drawer-polish. |
| Acceptans | Alla befintliga böner kvar, samma POSTbody utan offering; DOM ej composer + fysisk mutation; JS gröna; Firefox1280+390; inlinefeedback. |
| Stopvillkor | Compose ensam väg till bön → STOPP. Inventeringen visar ingen sådan bön. |
| Bevis | Baslinje först, inventering av faktisk serverdefault/CLI, native DOM/mutation, normalt Offer-flöde i rigg. |

Kedjegrind: bevisar begriplig och nåbar tempelhandling, inte en ändrad ekonomisk/bönemekanik.

## Inventering före implementation

| Väg | Request och grind | Slutsats |
| --- | --- | --- |
| Webb Perform → okRite → castRite | POST `/worlds/{w}/settlements/{s}/rite`, `{prayer:id}`, ingen offering. Knapp om !cooldown och affordable och ingen aktiv frenzy. | Befintlig traditionell väg för varje visad bön. Rename Offer, samma body. |
| Webb composer → okRiteComposed → castRite | Samma endpoint/prayer, tillägg `{offering:{good:qty}}`; tom composer faller tillbaka till okRite. Composer bara när !cooldown och affordable. | Inte ensam väg till någon bön; tas bort från webb. Aktiv frenzy har ingen Performknapp men är redan aktiv, inte en exklusiv bön. |
| Server SettlementHandler.Rite | settlement.go998 använder body.Prayer, bara tomt id får defaultBattleFrenzy. 1109: initialt används scaleOffering(spec.Offering, default multiplier1); 1125: composed använder explicit body.Offering. | Samma bön och recept nås utan compose. Regler orörda. |
| Keryx rite | cmd_rite.go204 explicit --prayer; utan --offer inget offeringfält; 213–222 explicit --offer -> body.offering. --offer-multiplier också kvar. | Båda vägar finns; AI-yta behåller --offer. Inget dubbelt kommando, ingen CLIändring. |
| Katalog | religion.PrayerSpecs och CulturePrayers: tre böner per kultur, alla har traditionellt Offering; province.go536 affordability läser tempel/kharis/receptstock. | Samma val, gates och serverkatalog kvar. |
| Inlinefeedback | kult.js använder redan showInlineResult efter rite, inga alert-träffar. | Orderns fyra alert är en stale basuppgift; tidigare slice hade redan konverterat dem. Bevara och prova inline. |

**Claude 2026-10-12 11:42:** tolkningen godkänd: **en Offer per befintlig bön**, inte en gemensam tempelknapp (den skulle kräva ny bönemekanik). Compose helt borta ur webben; Keryx --offer kvar. Ingen STOPP-konflikt.

## Grindar

| Grind | Resultat/bevis |
| --- | --- |
| Kod | [598 bas-JS](baseline-js.log), [598 full JS](full-js.log); server/CLI-diff noll, inga nya migrationer. Go-svit behövs inte för denna webbslice. |
| DOM | Före: tre .offer-composer + varufält. Efter: **0 composer, 0 numberinputs** i riktiga browser-DOM:en. Antal Offer följer aktuell servercatalogs affordability/cooldown, ingen bön borttagen. [Fakta före](baseline/facts.json), [efter](after/facts.json). |
| Mutation | Kompositör fysiskt återinförd i kult.js → **composed offering leaked into DOM**; originalbytes återställda och native DOM grön. [Sammanfattning](dom-mutations.log), [röd](mutation-dom.log), [grön](restored-dom.log). |
| Semantik/användare | Native Offer på Potnia's Vision skickade **bara `{prayer: minoan_oracle_deposits}`**, HTTP200. Ingen offering/offer_multiplier. Servern svarade **The gods are silent. Your offering was received, but they do not answer.** Inline-raden finns bredvid bönerna. Det är ett accepterat offer med negativt böneutfall, inte HTTP-fel. [Action och verkliga API före/efter](after/facts.json). |
| Bild | Firefox1280+390 med Mycenae och Nestor of Mycenae; inga JS-fel. [Desktop](after/desktop.png), [mobil](after/mobile.png), [inline efter Offer](after/desktop-result.png). |
| Process | **33253f3/mig173**. [Runtime](rig/runtime.json), [provenance](rig/provenance.txt), [bygglogg](rig-build.log), [samlat bevis](verification.json). Cachad acceptansruntime + lokalt CGO0-bygge, current migrations/worktree-webb; kult/main-assets bytejämförda. |

## Bild- och handlingsfixtur

Det ursprungliga aktiva Eirene/Mycenae saknade tempel och hade kharis1; [API-baslinje](baseline-api.json). För att pröva tillgängliga offer används en **egen nyregistrerad testspelare**, normalt ansluten till befintlig aktiv isolerad värld **3b5fb9cd**. En explicit SQL-fixtur tillför bara denna spelares Mycenae, tempel, kharis och lager; servern visar tre affordability=true. [Scenario](scenario.json), [start-API](fixture-api.json), [setup](fixture.log). Ingen annan spelares rader skrivs om.

Detta är inte ett naturligt tempelbygge/produktionsflöde. **Offer-handlingen själv** går från normalt login genom riktiga knappen, API och inline, utan SQL-skrivning eller browserpayload-/FOW-override. Arkiverade världar avvisar POST med409 och används därför inte för handlingsbeviset; den första arkiverade layoutfixturen kan inte bevisa ett fungerande Offer. Godernas svar avgörs av oförändrad servermekanik.

De publicerade **förebilderna** använder verkliga JS-moduler ur **53d33910**, levererade genom browserinterception, mot samma kompatibla **33253f3-backend** och samma nya testspelare. Detta byter UI-källa, inte data. [Desktop före](baseline/desktop.png), [mobil före](baseline/mobile.png). Förebildernas UI har Perform och kompositör; efterbildernas UI har Offer och inga kompositionskontroller. Ursprunglig processprovenance939c9d9 finns separat i baseline-provenance.txt.

## Ändring och avgränsning

Kompositörens extra goodsread, estimat/favours/worth-rader, inputbyggare, cast-composed-handler och main.js globala kopplingar tas bort tillsammans. Traditionell offertkostnad visas fortfarande. Daily temple-stock/mood/net, kultnivå, alla böner samt cooldown/affordability/frenzy-gates finns kvar. Endast etiketten Perform → Offer ändras på handlingen.

Orderns fyra alert() var redan konverterade i basmaster: kult.js har inga alert-träffar och använder samma showInlineResult för svar/fel. Den befintliga inlinevägen bevaras och riktigt svar verifieras. Ingen ändring av böneregler, chans, khariskostnad, serverkatalog eller Keryx. Därmed finns ingen STOPP-konflikt om en exklusiv kompositionsbön.

[Verktygsindex](../../../tools/README.md) beskriver fixture, två UI-källor, DOM-check, mutation och aktuellt runtime. En ny fixture behövs om även Offer-klick ska köras om efter cooldown; rena DOM-checks kan upprepas utan action.

Metodisk lärdom: en läsbar archived-värld är en layoutfixtur, inte ett handlingsbevis; ett lyckat request måste prövas på en aktiv värld. Ingen naturlig tempelgrundning eller full tre-browser-paritet påstås.

**HASH + STOPP:** Claude granskar bevis och bilder, integrerar och deployar.

## Granskningens CSS-städning

Claude12:08: den borttagna kompositörens `.offer-*`-block var kvar. Hela blocket och dess kommentar
är borttaget i samma gren; sökning av klasser/selectors i web/templates/JS hittar inga andra produktionsbrukare.
Trade-flaggor/element-ID med offer-good är inte dessa CSS-klasser och berörs inte.
Ett befintligt kontrollverktyg, late_dom_probe, väntade däremot på den borttagna varuinmatningen;
`cult-reload` följer nu den riktiga Offer-knappen och provar samma fokusbevarande vid sent svar.
[Riktat Chromium-DOM-prov](late-dom/results.json) grönt, [598 JS omkörda](full-js.log) gröna.
Efterbilderna visar samma synliga UI; borttagna regler saknade matchande produktionsnoder.
Serverad slut-CSS är bytejämförd, SHA i verification.json; runtime.jsons ursprungliga CSS-SHA avser bildbygget före städningen.
