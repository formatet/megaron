# Öppningsdispatch

## Sammanfattning

Nya och återvändande Wanaxer får en personlig, beständig introduktion när deras nya Host skapas.
Samma arkiverade text visas i Keryx, webbens dispatchfönster och Notifications; Codex `welcome`
förklarar notisen. Produktionscommit `a3ba0d39`, rebased från `a7b8c858` till master `a60995d0`.
Ingen migration. **LIVE `b2d31496`**, healthz `{commit:b2d3149,migration:179,status:ok}`.
Server PID 155119 startade 2026-10-14 17:50:23. Efterdeploykvittona läggs i en separat docscommit.

Kedjegrind: **blockerar** den nya spelarens orientering. **TEXT** prövas av en främling i playtest;
begriplighet är inte bevisad av utföraren. Mobilens en-skärm-krav är däremot uppmätt och kontrollerat vid 1:1.

## Slice-kontrakt

| Fält | Kontrakt |
|---|---|
| Problem | Ny eller återvändande Wanax möter världen utan kort introduktion till tid/mat/gudar/fysiska order/brons. |
| Spelarsanning | En personlig arkiverad text visar aktuella regler med källornas verkliga tal. |
| Invariant | Exakt en notis per ny Hostliv; idempotent upprepad/samtidig join/rise; inga gamla event/notiser tolkas om. |
| Scope | join/rise-transaktion, gemensam textbyggare, graceaccessor, Keryx/webbnotis/Codexartikel, prov. Ingen migration. |
| Non-scope | Registreringens öppnande, världsskapande, balans eller ny visuell familj. |
| Acceptans | En arkivrad vid join, egen rise-text, återbesök ingen extra rad; tal från konstanter/runtime; vanliga formatterare inget råJSON; högst en skärm. |
| Stopvillkor | Någon aktuell regel behöver väljas om för att texten ska vara sann. |
| Bevisplan | Färsk handlerbaslinje; faktisk join/rise/dubbeljoin/retry/rollback; talmutationrött→återställtgrönt; fullGo/vet/JS; naturlig joinnotis i CLI/Firefox1280/390. |

Kedjegrind: blockerar en ny spelares orientering. TEXT i playtest; ingen BILD.
Orderns äldre soldtimer finns inte längre: Hostreserver dräneras inte före
founding. Texten visar fasta aktuella mängder och beskriver subsistens.
Olivewoodland tar1,5 ticks, cederskog/hills2; texten skiljer dem åt istället
för orderutkastets förenklade alla-skog2. Talen läses från rörelsetabellen.


## Baslinje

Den bevarade WIP-kopian innehöll redan implementeringen och riktade prov från 2026-10-13.
`baseline.log` är därför en tidig WIP-baslinje, **inte** oförändrad master.
`master-baseline.log` är den nya riktade join/rise-baslinjen på oförändrad `a60995d0`, separat tom PG16,
migration 179. Slutlig full Go kördes på en annan tom PG16 (`full-go.log`).
Nativeprocesserna svarade själva med healthz `{commit:a3ba0d39,migration:179,status:ok}`;
full provenance finns i respektive `verification.json`, och loggarna visar tick-worker och faktiska API-anrop.

## Experiment och resultat

| Hypotes/prov | Resultat |
|---|---|
| Samtidig join eller retry får inte skapa två liv/notiser | Riktig handler/PG: en 201 + en 200; exakt en arkivrad. Rise + retry ger en egen rad. |
| Notisfel får inte lämna ett halvt Hostliv | Avvisande SQL-trigger: join 500, inga Host/enheter/player-world-records; retry efter återställning 201. |
| Notisen följer källorna | Runtime 1 sekund/1 minut/30 minuter/1 timme; grace, tempeltid, rörelse, resurser och kollapströskel hämtas ur aktiva källor. |
| Avsändarens arkiv är privat | Annat registrerat konto får noll OpeningDispatch-rader via verklig API. |
| Klienterna tappar inte sista stycket | Fysisk webb- och Keryx-mutation ger namngivet rött test, exakta originalbytes återställs, färskt grönt prov. |
| Naturlig join är läsbar | `native/`: register/join/retry, CLI och Firefox 1280×900/390×844 via chip och arkiv, 30 minuter/tick, starttröskel 4. |
| Naturlig Rise fungerar | `native-rise/`: 1 sekund/tick, starttröskel 2; register/join/found, placera timmer/sten, bygg kasern, rekrytera tills övermobilisering ger population 1 + collapsed/dispossessed, verklig worker; Rise + retry ger totalt två notiser, exakt en returning. CLI och båda Firefoxbredderna visar Begin again. |

Riggens första försök hade två felaktiga antaganden: Keryx får visa sitt etablerade kind-namn i rubriken,
och en läst notis finns inte kvar i strippen när nästa viewport öppnas. Slutprovet kontrollerar att
**payloaden** visas utan rå JSON och använder separata nya spelare för chip-proven; återkomstens mobilprov
nås via det bestående arkivet. Ett första återkomstförsök läste fel statusyta; det korrigerades till en
READ ONLY-observation av den riktiga kollapsen. Inga produktionsändringar behövdes för dessa riggfel.

## Grindar

| Grind | Bevis/status |
|---|---|
| Kod | Full färsk Go/mig179 grön (world 91,586 s), `go vet ./...` grön, 623 JS gröna utan skip. |
| Funktion | `TestOpeningDispatchJoinConcurrentRetryRiseAndPrivacy`, `TestOpeningDispatchFactsFromActiveRules`, `TestOpeningDispatchArchiveFailureRollsBackHost`, gemensamma klientformatterare. |
| Mutation | `mutate.py`: missing-archive, hardcoded-grace, web-missing-paragraph, keryx-missing-paragraph: namngivet rött → identiska bytes → separat färskt grönt. |
| Visuell | Båda texterna vid 1280/390; hela texten och kontrollerna ryms utan vertikal/horisontell overflow. Ingen ny CSS/visuell familj. Bilder öppnade vid 1:1. |
| Semantisk | Subsistensreserver dräneras inte; återkomst behåller gudaställningen; Runner gäller order till fältenheter; arkivet behåller startögonblickets fakta. Ingen FOW-/notisläcka eller gammal eventtolkning. |
| Användare | Naturlig join och naturlig kollaps/Rise på egen riktig server/PG/Redis, båda klienterna; inga SQL-skrivningar eller HTTP/payload-overrides. |
| TEXT | Kvar i playtest: förstår främlingen tid, mat, tempel och bronsvägar? Vault §B bokför frågan. |
| Drift | LIVE `b2d31496`; ny process/healthz, mig179 dirty=false, granskade webb/Codex-bytes levererade. READ ONLY före/efter: alla tidigare eventpayloads identiska; befintlig Host silver480/rate0/calc150 bevarad. Air byggde om utan restart. Installerad Keryx `~/go/bin/poleia` byggd från verifierad källa. |

## Reproduktion

```sh
tools/gotest.sh
env -i HOME="$HOME" PATH="$PATH" go -C server vet ./...
node --test $(rg --files web/static -g '*.test.mjs')
python3 docs/reviews/oppningsdispatch/mutate.py
python3 docs/reviews/oppningsdispatch/live.py
OPENING_NATURAL_RISE=1 OPENING_NATIVE_OUT=native-rise python3 docs/reviews/oppningsdispatch/live.py
```

Nativeproven kräver Docker, Go, Python och Playwright Firefox. Gameplaykod måste vara committad och ren;
verktyget bygger egna binärer och använder egna PG/Redis-portar och en rensad miljö.
`native*/verification.json` innehåller API-notiser, viewportmått, fel-listor och den naturliga Rise-kedjan.

## Metodisk lärdom

En läst dispatch kontrolleras via arkivet; ett strip-prov kräver en oläst notis.
Full användarväg till Rise kan bevisas via övermobilisering utan att tillverka dispossessed i databasen.

## Kända avgränsningar

Ingen backfill till äldre liv. Gamla öppningsnotiser uppdateras inte när regler senare ändras.
Full introduktion visas i arkiv och `watch`; Keryx behåller sitt etablerade kind-namn i arkivrubrik.
En-skärm-provet gäller 1280×900 och 390×844. Begriplighet kräver främlingsplaytest.

## Deploykvitto

`deploy-health.json`, `deploy-process.log`, `deploy-assets.json` och `live-before.json`/`live-after.json`
visar provenance och bevarande. Den första hälsopollningen hann löpa ut medan air fortfarande byggde;
den gamla processen svarade då. Ingen restart eller DB-ändring gjordes. Slutkvitto togs först när
den nya processen svarade med `b2d3149`/179. Den väntande världen står kvar på tick150 och skapades
inte om; inga nya konton eller öppningsnotiser tillverkades på den delade världen.
