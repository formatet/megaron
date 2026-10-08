# N — Economy varurader (BILD)

Bas **master ba9e6faf** · gren **codex/economy-goods-rows** · arbetskopia
`/tmp/megaron-codex-economy-goods-20261008` · slutlig klientkod **d09b2cfb**.
**EN slice, ingen merge/push/deploy. Claude granskar/integrerar; därefter stopp.**

Automation har vara-select + antal i båda riktningarna, med **Add a good** och
**× / Remove good**. Minimum är önskat lager vid målet; Leave är det lager
som lämnas där när returlast hämtas. Noll, decimaler, flera varor och tom retur
bevaras. Add/Remove använder append och bevarar redan skriven indata.
Transfer hade redan vara-select + antal och behåller sin enkla good_key/quantity-body;
lagertal och skickad mängd i dess text skrivs nu i ord. Numeriska inmatningsfält
är fortsatt numeriska. Goods/Transfer/Automation/Wants och Crewed-by är kvar.

Varuvalen kommer från egna stadens fulla inventory, inklusive nollstock,
silver och parked varor. Offertens `/api/v1/goods` utesluter dessa och används
inte här. Cult är ej fraktbar. Misslyckad inventory-läsning visar inlinefel;
Automation öppnas igen för nytt försök. Ingen ny server-/API-/CLI-regel.
Crewed-by-markup, bemanningsvalets id-koppling, båda POST-blocken och route
list/pause/resume/delete är byte-identiska mot basen, se [scope.json](scope.json)
och `tools/economy_goods_scope.py`. Server, API.js, CSS, Diplomacy/Gossip orörda.
Codex routes/rough-edges beskriver de nya kontrollerna. Fyra ytor: webb + Codex
uppdaterade, server/keryx verifierade oförändrade då bara webbens inmatning ändras.

**Grind: proves** att den befintliga logistiska ytan kan användas utan developer-
syntax i brist→produktion-kedjan; **waits for** Claudes granskning/BILD/integration.
Ingen ny kanon eller ekonomisk regel, ingen väntan på Timothy (delegerat till Claude).
Detta bevisar inte hela geografi→brist→brons→elit-kedjan.

## Före kod och tester

Kontraktet committades **44773e09** före kod. **68dff89d** innehöll actual
createStandingOrder/drawer-repros: tre namngivna röda före produktionsändring,
samt bevarad spärr för samma stad/tom utlast; [repro-before.log](repro-before.log).
Oförändrad bas: **433 JS** gröna, [js-baseline.log](js-baseline.log).
Första riktiga basbilden och CSV-order201 togs före kod; denna partial-arm
fastnade senare på riggens felaktiga visible-väntan på en dold `<option>`.
Komplett baslinje kördes därefter om mot arkiverade, oförändrade ba9-assets.

Slutresultat **438 JS pass / zero fail** ([js.log](js.log)). N:s fem actual
handler/drawer-prov täcker multipla varor, noll/decimaler, båda bemanningsval,
tom retur, spärrar, bibehållna flikar, silver/parked/nollstock och fetch-fel/retry.
**Fyra fysiska mutationer** (utlastkälla, retur-noll, crew-id, floor) ger
namngivet actual POST-prov rött och restaurerad svit grönt; [mutations.log](mutations.log),
`tools/economy_goods_mutation.py`, återställning i finally.

**Full tools/gotest.sh: ny PG16, mig160, ALLA paket gröna**, world **93.236s**,
exit0 ([go.log](go.log)). Den gick före klientändringen på samma oförändrade
Go-träd; headern anger 68dff89d + rigg-WIP. Första körningen avbröts av /tmp-kvot
under kompilering och räknas inte, [go-quota-rejected.log](go-quota-rejected.log).
Omkörningen använde egen go-wrapper som bara exporterar GOTMPDIR under /home-
cache och execar /usr/bin/go; [go-wrapper.txt](go-wrapper.txt), rensad testmiljö,
färsk databas. Ren env-i **go vet ./... exit0**; [metadata.json](metadata.json).

## Riktiga slutarmar och BILD

[baseline/proof.json](baseline/proof.json): actual **ba9e6faf /160/ok**.
[after/proof.json](after/proof.json) och [repeat/proof.json](repeat/proof.json):
actual **d09b2cfb /160/ok**, samma fulla binär SHA256 **a830ce8743166f31…**,
source/bin hashes i metadata. Varje arm har ny egen PG16/Redis och normal
register/join/founding/settle, fem fysiska landmarscher med vanliga orderbud,
och vanlig kolonisering till två egna städer. Endast FOW-känd terräng används
för gångvägen; inget SQL, inga State-knowledge-fixturer eller teleport.

Alla tre riktiga webbar skickar **standing-order201** (grain200 + fish50,
retur silver0 + stone20, destination supplies crew) och vanlig **Transfer201**
(grain1). Request-fält, varuordning, antal, threshold/floor och crew-koppling
matchar baslinjen. [body-comparison.json](body-comparison.json) jämför båda
fångade POST-kropparna och URL:erna; **bara respektive worlds verkliga stads-/
provins-id normaliseras till samma roller**, eftersom nya världar har nya UUID.
Inga varor/mängder/fält sorteras bort eller normaliseras. HTTP-status lästa från
riktiga accessloggar. POST-blocken är dessutom byte-identiska i koden.
Efterarmarna provar extra Add→Remove i båda grupperna och bevarad indata,
alla fyra varor/noll samt faktisk CSS-färg. Zero page exceptions och
horizontal overflow på desktop1280×900 och mobil390×844. Input använder
CSS --text, inte blek text på ljus bakgrund. Dessa är presentations-/dispatch-
bevis; godsens slutleverans och automationens hela cykel är inte nya scenarier.

**BILD, 1:1 före/efter, desktop + mobil** (slutbilder inspekterade i original):

| Yta | Före | Efter |
|---|---|---|
| Automation desktop | [bild](baseline/automation-desktop.png) | [bild](after/automation-desktop.png) |
| Automation mobil | [bild](baseline/automation-mobile.png) | [bild](after/automation-mobile.png) |
| Transfer desktop | [bild](baseline/transfer-desktop.png) | [bild](after/transfer-desktop.png) |
| Transfer mobil | [bild](baseline/transfer-mobile.png) | [bild](after/transfer-mobile.png) |

Repeat har samma fyra bilder. Fyra varurader kräver vanlig vertikal scroll på
mobil; Create route nås och klickas i riggen. Kartorna i färska världar är olika,
inte pixel-identisk terräng. Oförändrade cargo-listans råa tick/koordinater är
utanför denna smala input-slice; bilden visar dem öppet, ingen bortklippning.

## Fynd, omtag och städning

8b2-after/repeat gick hela POST-flödet men visade bleka siffror från den
befintliga `.unit-row input`-regeln. **Superseded**, inte slutbevis.
Dcba-armarna återgav läsbar färg men deras verkliga viewport-probe var **röd
för nowrap-overflow**. D09 använder explicit radbrytning mellan label och
control i befintlig field/inline-fields; båda nya slutarmar gröna.
`tools/economy_goods_layout.py` kör riktig CSS och den faktiska renderern från
historiska commits, reproducerar båda visuella felen och kräver grön aktuell
rad vid båda drawerbredder; [layout.json](layout.json). Det är ett separat
render-konsumentprov, inte fabrikerat gameplay eller en fysisk källmutation.

Övriga rejected-loggar dokumenterar riggens fel terrain-enum, fel preview-URL,
courier_required som felaktigt behandlades som spärr, dold option-väntan,
frånkopplad/liten ö och för strängt spawn-filter. De räknas inte som godkända
armar. Slutriggen använder vanliga spawnförsök och BFS över endast känd,
passerbar terräng, ett steg åt gången; serverns verkliga budresor inväntas.

Alla egna processer/containrar avslutade. Egna riggar/binärer och Go-temporärer
städade först efter beviskopiering: **160,367,293 bytes**, plus **65,847,996 bytes**
early dubbletter/partial-binär. SHA/path/bytes i [own-cleanup.json](own-cleanup.json)
och [early-cleanup.json](early-cleanup.json). Inget annans, inga gamla smutsiga
worktrees eller delad Go-cache berörda.

## Resume / verifiera

Stå kvar på denna gren; ny utveckling kräver nästa uttryckliga order från Claude.
`tools/README.md` beskriver live, mutation, scope och layout-prov.
För live: ny egen OUT, full env-i build från server med `main.buildCommit`
satt till d09b2cfbc8afebcc971e2cdd27f5e999188e9172; GOTMPDIR får ligga i egen
/home-cache. Bas-armens web fås med git archive ba9e6faf web; kör baseline mot
den och after mot denna grenkod, varsin fresh PG16/Redis. Kör aldrig fysisk
mutation medan en livearm läser källorna. Binaries är städade, bygg nytt.
Hash till Claude och STOPP; ingen merge/push/deploy.
