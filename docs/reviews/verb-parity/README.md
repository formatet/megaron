# Paritetsrapport fyra ytor

## Kontrakt före kod
Problem: manuella verblistan saknar reproducerbar kodkorsning.
Spelarsanning: rapport synliggör befintliga ytluckor, inga verb ändras.
Invariant: bara läsning; metod + rutt och källevidens, dynamiska oklarheter synliga.
Scope: tools/verb_parity.py, explicit allowlist/aliases, parserfixturtester, verktygsindex och rapport.
Non-scope: produktionskod, DB, ändring av verblistan eller auto-fix av luckor.
Acceptans: chi-nästning, CLI-format/variabler, webbkonkat/template/metod,
Codex-omnämnanden, allowlist och verblistadiff kan återges med parserprov;
första körning med rangordnade, manuellt kontrollerade fynd.
Stopvillkor: spelkanon/produktionsförändring krävs, då bara fynd till Claude.
Bevis: unittest-fixturer, fysisk parsermutation, riktig körning och källkorsning.
Kedjegrind: bevisar att handlingar har nåbara klientytor; statisk rapport
bevisar inte runtime, upptäckbarhet eller att artikeltexten är begriplig.

## Resultat
På produktionsbas `1e1fcad4`: 112 API-registreringar, 23 dokumenterade
undantag och 89 kvar i endpoint-tabellen. Keryx har statisk anropsevidens för
78, webbmodulerna för 83, och 87 har avgränsade Codex-omnämnanden.
Ingen olöst konkret request kvar; 10 generiska transport-/assetanrop redovisas
separat. Första fulla körning: [report.md](report.md), all evidens/inputfingerprint:
[report.json](report.json). Normaliseringssyntax och shorthand finns i verktyget;
undantag och artikelalias i `tools/verb_parity_allowlist.json`.

### Tio prioriterade yta-/registerluckor, källkontrollerade
Det finns **tre verifierade saknade spelarytor**, därefter en fråga om en
åtkomlig serverrutt, fem registerluckor och en ren endpoint-lucka. Dessa tio
är inte tio påstådda blockerade spelarflöden. Ingen produktionsfix beställd här.

| Prioritet | Fynd | Källbevis och nästa steg |
|---|---|---|
| 1 | Värdens skickade brev saknar webb-läsyta före grundning | `ui/drawers/diplomacy.js:147` hämtar inbox, men `:148` gör outR=null utan MY_SETTLEMENT_ID. Server `messenger.go:701` har GET founding/messengers och Keryx `cmd_messenger.go:280` läser den. Verifiera spelarresa och gör webbens host-outbox i egen slice. |
| 2 | Abandon saknar Codex-instruktion | POST-rutt, `cmd_abandon.go:14`, webb `war.js:696` finns. Alla Codex-artiklar sökta: endast ordet abandoned om en förstörd hamn i sea.md, ingen instruktion/konsekvens för verbet. Skriv artikelstycke i colonies/city efter mekanikens kontrakt. |
| 3 | Call-back saknar Codex-instruktion | `main.go:497`, webb `diplomacy.js:520` och CLI-anrop finns. Ingen call-back/call back i artiklarna, sea beskriver sjöväntan men saknar denna handling. Dokumentera egen-hamn-regeln och utfallet. |
| 4 | POST /worlds finns utan spelar-/adminyta och utan uttrycklig admincheck | `main.go:367` har vanlig auth.Middleware; `WorldHandler.Create` från world.go:156 har ingen rollkontroll. Båda klientytor saknar anrop. Ska denna vara administration eller ett spelarverb? Claude/Timothy avgör, ingen kod ändrad. Den är därför inte gömd i adminallowlist. |
| 5 | GET Agora saknas i verblistan | `main.go:342`, `cmd_agora.go`, webb api.js och community-chat.md finns. Registret nämner inte automatkontots läsverb. Lägg registret i rätt status i separat dokumentationsslice. |
| 6 | Agora password saknas i verblistan | `main.go:343`, CLI cmd_agora.go do(MethodPost), webb requestAgoraPassword och artikel finns. Det är en annan rutt än befintlig auth/password. Samma registeruppdatering som 5. |
| 7 | Founding message saknas i verblistan | POST founding/messengers finns i main.go:431, CLI skickar från host, webb render/map.js:4485 väljer hostens sendPath. Registrets message-rad nämner bara settlement-rutten. Lägg founder-varianten i samma rad. |
| 8 | Read-all saknas som muterande registerverb | POST notifications/read-all finns och används av CLI/webb. Registret listar notifications bland läsytor, inte denna mutation. Registrera beteendet utan att blanda ihop det med den avsiktligt borttagna archive-delete-knappen. |
| 9 | Enskild read saknas som muterande registerverb | POST notifications/:id/read används av web/chips.js:102; inget direkt Keryx-anrop (CLI använder read-all). Dokumentera per-item kontra all-semantiken i registret; frånvaro av samma endpoint är inte bevis för att CLI saknar notifieringsläsning. |
| 10 | Handelsbar global varukatalog saknar direkt CLI-anrop | GET /api/v1/goods från GoodsHandler.TradeableCatalogue används av webb Diplomacy; Keryx anropar bara stadens /goods. Kontrollera om ett catalogue-läge behövs för skript/offerter. Inte en fastslagen trade-blocker: CLI kan redan skicka offerter. |

### Andra utslag som inte är bevisade förmågeluckor
GET /worlds saknas bland webbmodulernas anrop: lobby/auth lever också utanför
modulerna. Auth register/login och join är därför uttryckliga templateundantag.
GET /settlements och /settlements/:id motsvaras delvis av cities/provinces/city.
Webbens byggnadsläsning kommer i större province-payload i stället för /buildings.
Map-API:erna marches/messengers/rural-projections och den äldre GET province/trade
saknar vissa direkta klientanrop; verklig informationsskillnad måste mätas mot
units/sightings/inbox/outbox/trades innan en ny klientfunktion beställs.

Registrets batch POST /units/recall finns inte på denna master, men radens egen
not anger aggregatgren och kvarvarande BILD-grind. Det är ett **känt väntande
registerpåstående**, inte en ny regression. Keryx/webb/Codex för vanlig recall finns.

### Bevis och begränsningar
13 parserfixturer gröna: chi-nästning/Group/With/helper, kommentarer, query,
format- och item-URL, cross-file Go-helper/factory-parametrar, JS konkat/template,
multiline ternary, injected URL-objekt, regexp med citattecken, lexical shadow,
shorthand method, dynamisk okänd metod och bounded artikelalias.
`python3 docs/reviews/verb-parity/mutate.py` ändrar fysiskt shorthand PUT/DELETE
till GET: namngiven fixtur ger AssertionError, finally återställer och alla
13 blir gröna. Logs /tmp/megaron-parity-{tests-final,mutation-final}.log.
Två riktiga markdown-körningar byte-identiska; normal kommandokörning utan DB.
Ingen ny Go/JS-produktion: full Go-svit körs inte för det fristående Pythonverktyget.

Detta är en begränsad källparser, inte Go/JS-kompilatorn. Finita factory-/helper-
parametervärden och injected URL-properties är en statisk överapproximation;
markeringen betyder hittat anrop med ruttform, inte att varje gren alltid nås.
Okända metoder räknas aldrig som GET. Artikelalias är uttryckliga och begränsade
till rätt artiklar; omnämnande betyder inte korrekt, fullständig eller begriplig
instruktion. Nya språkformer visas som olösta eller kräver nya fixturer.
Inget runtime-/FOW-/upptäckbarhetsbevis och inga nya ytor implementerade.
Verblistan är läst och **orörd**. Ingen merge/push/deploy från Codex.

## Resume checkpoint
Gren codex/verb-parity, /tmp/megaron-codex-parity-20261007, produktionsbas1e1fcad4.
Kontrakt971635b4. Recept:

```
python3 -m unittest discover -s tools -p 'test_verb_parity.py' -v
python3 docs/reviews/verb-parity/mutate.py
python3 tools/verb_parity.py
python3 tools/verb_parity.py --json
```

Verblistan default ~/Dokument/myltavault/megaron_verblista.md; annan fil via
--verblista, annan checkout via --root. Saknad registerfil ger explicit fel.
Claude granskar parser/allowlist och de tre första ytfynden innan nästa slice.
