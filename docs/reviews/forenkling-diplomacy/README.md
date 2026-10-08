# M — Diplomacy, BILD

## Sammanfattning

Fyra flikar blir **Correspondence + Known**. Första brevet flyttas från Compose
 till **Known → Write → öppnad tråd**; befintlig tråd behåller sitt oförändrade
brev-/köp-/säljformulär. Known förenar samma Cities- och Rulers-svar, med namn,
ägare, kontaktstatus och Write; fyndigheter/plats finns under Details. Tal skrivs
i ord, även det redan avrundade ryktesavståndet. **Ingen merge/push/deploy.**
Slutlig klientkod: `2527a23bfb62de0187fdcd74a719b013852dd116`, bas master `6d8d5f60`.
BILD lämnas till Claude enligt nattens delegation; ingen Timothy-väntan.

| Bevis | Utfall |
|---|---|
| JS före / slutversion | 425 / **433 pass, zero fail** |
| Repro före produktionskod | fyra namngivna faktiska drawer-prov röda vid `70ecaebf`, sedan gröna |
| Fysiska mutationer | extra Compose / saknad draft / rumour-write / NaN-tid / stale composer / sent actions-svar / rått antal: var och en named röd → återställd grön |
| Full färsk Go + vet | `tools/gotest.sh`, ny PG16/mig160, alla paket (world91.470s); ren env-i vet exit0 (explicit statuslogg). Go på0f18d815; Go-trädet identiskt i slutversionen |
| Skyddad scope | server/keryx, API, visibleOrigins, Inspect, gossip.js och CSS tomdiff; befintliga trade/reply/dispatch/passage/callback-funktioner + inline trade-form byte-identiska |
| Baslinjens riktiga webb | `/healthz` **6d8d5f6056f3c9dbc1e5397dfa30b6f34451cb28 /160/ok**; Compose-brev201, Reply200 och fysiskt återvänt läsbart svar, buy+sell201, båda erbjudandena lästa, Inspect201, zero browser errors |
| Slutversionens webb | **två rena fresh PG16/Redis**, actual health2527a23b/160/ok, brev201/Reply200/återvänt svar/buy+sell201/Inspect201; inga browserfel/overflow |

## BILD och provenance

Före: [Known/Cities mobil](baseline/known-mobile.png), [Rulers](baseline/rulers-mobile.png),
[Compose](baseline/first-letter-mobile.png). Efter: [Known mobil](after/known-mobile.png),
[Known desktop i kartkontext](after/known-context-desktop.png),
[tom skrivbar tråd](after/first-letter-mobile.png),
[återvänt svar](repeat/returned-reply-mobile.png), [köpform](after/trade-buy-mobile.png),
[säljform](repeat/trade-sell-mobile.png). Alla finns även i desktop/390 kontext.

Baseline [proof.json](baseline/proof.json) bär actual6d8d5f60/160/ok.
Slutliga [after](after/proof.json) och [repeat](repeat/proof.json) bär båda actual
**2527a23bfb62de0187fdcd74a719b013852dd116 / migration160 / ok**; identisk hel binär
SHA256 **d75c682f52d2bb3bfc873e9df631cc35fc511f45ea7c1620de9e053cca483165**.
After: 3 marscher/14 game days; repeat: tre marscher/tretton game days.
Båda klickar först härskarens, sedan stadens Write, skriver ett riktigt första brev,
läser mottagning och skickar Reply från mottagarens browser, väntar på faktisk
homecoming och läser reply_text. Köp/sälj skickas i tråden med riktiga escrowvaror,
läses i mottagarens tråd med båda Accept/Decline-knapparna; separat Inspect-brev201.

## Slice-kontrakt


Problem: fyra flikar delar samma diplomatiska ingång och första brevet kräver Compose.
Spelarsanning: Correspondence för brev/erbjudanden/svar, Known för kända härskare och städer samt Write till samma tidigare kontaktbara destinationer.
Invariant: inga förmågor eller data/FOW-gates ändras; inga rumour-only-destinationer öppnas för dispatch.
Scope: diplomacy.js yta/navigation, main.js bindings, Codex och bevis.
Non-scope: gossip.js, trade-form/body/acceptance logic, server/API/keryx, regler och CSS.
Acceptans: exakt två flikar; kända städer och härskare läsbara med tal i ord; första brev via Known, fortsättning/handelsförslag i tråden, Inspect och Reply bevarade; fel/rumour ger ingen ny dispatch; riktiga brev+köp/säljförslag+svar före/efter.
Stopvillkor: informationsgating/serverfel eller förmåga som inte går att bevara inom ytan rapporteras till Claude.
Bevis: baslinje6d8d5f60 före produktion, faktiska drawer-handler JS regressionsrött, mutation, full JS/fresh Go/vet; var sin fresh PG16/Redis med vanlig register/join/settle/march/browser och healthz, desktop1280 och390 BILD, två rena efterarmar.
Gate: bevisar nåbar brev-/handelsyta för kedjegrinden, inte en full bronskedja.

## Baslinje och skrivvägar

Oförändrad klient/suite och de första desktop/390-bilderna togs före produktionsändringen.
Första kompletta scenariodumpen behövde rättad rigg; den godkända baslinjen kör
samma arkiverade oförändrade6d8d-assets och renbyggda6d8d-server på **egen färsk DB**.
Ingen delad DB eller public livevärld används.

| Tidigare väg | Efter M |
|---|---|
| Compose: första brev, valfri tidigare kontaktbar destination | Known: stadens Write (eller härskarens Write till en sådan stad) öppnar en tom tråd med samma destination |
| Compose: köp-/säljbilaga redan i första brevet | Samma befintliga inline-trådsformulär, även i den tomma tråden; båda riktningar/body verifieras i faktisk dispatch-konsument |
| Trådens nya brev / köp / sälj | Oförändrade formfält, villkor, handlers och API-body |
| Incoming letter Reply / läs svaret hemma | Oförändrat; vanlig fysisk resa och Reply200 reproduceras i webb |
| Inspect: brev från stad eller Host | Oförändrad render/map.js; stadsbrev201 i M-riggen, Hostvägen i oförändrade tidigare Inspect-bevis/full JS |
| Trade accept / decline / cancel, passage / call-back | Alla befintliga funktioner och trådens block kvar, byte-identiska; full svit. M-webb visar två Accept/Decline-kontroller, skickar inga beslut |

Write använder **exakt tidigare Compose-predikat på State.provinceData**.
`/cities` (som också innehåller rykten) är ingen dispatch-katalog och utökar inte
kontaktbarheten. Rumour/own/outpost/namnlöst/saknad destination kan inte skapa en
ny draft. Servergating och API-vägar är orörda. Host får ingen ny settlement-only
trade-composer; dess verkliga brevväg via Inspect kvarstår.

## Experiment

| Fynd / hypotes | Resultat / beslut |
|---|---|
| Tom första tråd har ingen latest-timestamp | BILD visade NaN ago; named rött prov, sedan **New conversation**. Superseded0f18-armen räknas ej slutbevis |
| Snabba ruler-Write→city-Write kan visa en gammal composer medan fetch pågår | Riktig repeat visade inget POST efter förlorad text; named repro rött, sedan Loading tills vald tråd är färdig |
| Sena capability-hints återskapar hela skrivytan med innerHTML += await | Andra realwebbrepro och nytt delayed-actions/typed-letter-prov rött. insertAdjacentHTML append bevarar redan inskrivna ord. Båda Write-knappar och fulla flödet körs om på2527a23b |
| Reply hemma skulle heta returned | Fel riggantagande; serverns verkliga status är **arrived**, med reply_text. Första baselinearm förkastad |
| Spara Reply-response först efter en page reload | CDP-body finns inte längre; svaret fångas direkt efter Reply200. Förkastad arm nådde funktionella assertions men saknade giltig slutdump |

## Grindar

Kod och semantik gröna enligt prov och scope-audit. Bilder bedöms vid1:1 i normal
spelkontext desktop1280×900 och390×844. Inga nya CSS-klasser/färger; klientens
karta/FOW/chrome lämnas orörda. Randomiserade freshvärldar gör pixelidentitet
mellan armar irrelevant; jämförelsen gäller samma drawer-skala och användarvägar.
BILD kräver Claudes granskning före integration. TEXT begriplighet hör till playtest.

## Recept

Baslinjeassets kan återskapas med `git -C <worktree> archive 6d8d5f60 web`
till ett separat OUT och extraheras där; inga branchbyten i huvudträdet.

Från eget worktree, toolsREADME beskriver fullbyggd temenos med ren env-i och
`-ldflags '-X main.buildCommit=<fullhash>'`. Varje arm skapar PG16+Redis och städar
sina egna processer/containrar i finally. Ordinary register/join, readonly landmass
ur **egna** PG för att välja två närliggande spawns på samma ö; sedan enbart vanlig
Host-marsch-preview/POST, grundning, HTTP och riktiga browserklick. Ingen SQL-skrivning,
teleport, DOM-injicerad spelkunskap eller State-fixtur i webbriggen. Kamerapan är
endast presentation för verkliga klick. JS-proven stubbar HTTP/DOM uttryckligen.

```sh
node --test $(rg --files web/static/js | rg '\.test\.mjs$')
python3 tools/diplomacy_mutation.py
python3 tools/diplomacy_scope.py
tools/gotest.sh
python3 tools/diplomacy_live.py OUT BUILD_COMMIT baseline /path/to/archived/web
python3 tools/diplomacy_live.py OUT BUILD_COMMIT after
```

## Metodisk lärdom

- En första tråd saknar både brev och tidsstämpel: prova det tillståndet som egen ingång.
- Async navigation måste ta bort den gamla skrivytan; sena kompletteringsrader får inte återskapa redan skrivbara fält.
- Spara nätverkssvar när de kommer och använd faktiska serverstatusar i bevisapparaten.

## Kända avgränsningar

Fresh webb visar vanlig landkommunikation mellan två grundade spelare. Rumour-only,
misslyckad directory-fetch, Host utan city-origin och felaktiga destinationer provas
med stubbad HTTP i actual-handler-proven; inga sådana livefall fabriceras. Sjöpassage,
trade-accept/decline/cancel och slutlig godsleverans är inte nya realwebbscenarier här.
Handelsformulärets befintliga texter/nummer/tempo behålls i denna smala slice enligt
ordern om orörd handelslogik; att göra det till en mening är en senare slice.

Superseded rena17299/ec153-paket är separat märkta och räknas inte som slutbevis;
failureloggar ligger i rejected/. Tidigare musikarkiv/binärdubbletter och färdiga
M-riggar städas med paths/bytes/SHA-audit, inga andra agenters filer berörs.

**Sidofynd, endast mönster — ej reproducerade buggar:** City garrison `city.js:405`
och War recruit `war.js:270` använder också `innerHTML += await renderLockedActions`.
De är orörda; eventuell förlorad kontroll/text kräver egen repro och Claudes nästa order.
