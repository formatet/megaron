# Slice12 — live eyes på en transaktionsanslutning

Bas ed4e8804, codex/eyes-tx. Kod a6f12654 (drain+close) +5828cd43 (Claudes loggtillägg).
Claude har mergat/deployat som51e805a4 (inkl5828cd43); detta är beviskvittot. Exakt prod500 återkom inte i privat snapshot, varken
sekventiellt eller med4parallellaPOST (båda armar201×1/409×3). TX-felet är namngivetRED→GREEN.

| Kontrakt | Innehåll |
|---|---|
| Problem | PlaceGubbe ger500 efter marsch/bud eftersom LoadLiveEyes frågar DB med en öppen resultatuppsättning på samma TX. |
| Spelarsanning | Placering fungerar även medan egna enheter och bud är på väg; samma liveögon på pool och TX. |
| Invariant | Alla rader samlas och stängs före nästa DB-fråga; samma interpolation, passagestatus och FOW. |
| Scope | Tre läsloopar i province/eyes.go, Queryer-audit, Place/Unplace500-diagnostik (tillägg07:44), riktiga PG/HTTP-regressioner och bevis. |
| Non-scope | Ny synlighet/radie, pathfindingregler, placeringslås, API/UI/CLI/Codex-kontrakt. |
| Acceptans | Marscherande enhet/utgående/återvändande bud ger201; TX frisk; pool/TX samma ögon; kvarvarande poolsvältprov grönt. |
| Stopp | Fix kräver ändrad FOW eller annan spelmekanik. |
| Bevis | Kompilerande masterRED, tre loopmutationer +tre loggmutationer namedRED→exakt återställning→GREEN, fullfreshGo/vet/JS och naturlig placering efter verklig marsch. |

Blockerar spelarens placering/geografi→brist-kedja. Regression från slice11.

## Diagnos och baslinje

`LoadLiveEyes` anropade FindPath medan units/outbound/returning-resultatet ännu lästes.
Poolen kunde tidigare använda en annan anslutning; en riktig TX kan inte göra det.
FindPath behandlar databasfelet som best-effort och låter ögat stanna vid ursprunget.
Det bryter interpolation och svälter en pool med bara en anslutning.

**Viktig bevisgräns:** produktionens `could not lock hex (HTTP500)` är rapporterat av Arete
runda2 och diagnosticerat av Claude. Den lilla HTTP-fixturen gav201 även på master;
vi påstår därför inte att den återger just500. Även färsk livekopia nedan gav201/201. Baslinje `ed4e8804/183` är däremot
kompilerande, namngivet RED för tappad TX-interpolation i alla tre loopar
(`baseline-tx.log`) och separat MaxConns1-poolsvält (`baseline.log`).

## Ändring

`province/eyes.go` samlar giltigt skannade rader i tre typade slices och stänger varje
resultat före interpolationen. Existerande Scan-fallback, unitfilter, passage-status,
återkallat buds startport, tidsfönster, ögonslag och havshorisont är oförändrade.
Ingen extra poolanslutning eller flytt av FOW ut ur placeringens transaktion.
Ingen migration, omräkning eller klient-/API-/Codexändring behövs: befintliga fyra ytor
har samma kontrakt och blir konsumenter av den fungerande serverläsningen.

## Queryer-audit

| Funktion | Fynd/beslut |
|---|---|
| LoadLiveEyes units/outbound/returning | Tre verkliga nästlade DB-anrop; drain+Close före FindPath i samtliga. |
| LoadLiveEyes settlement-loop | Skannar/append endast, stänger före unitfrågan; orörd. |
| loadRememberedTiles (world.go) | En UNION-fråga, endast skanning till set; deferred Close vid retur före nästa callerfråga. Orörd. |
| LoadHexHolds (economy/hex_take.go) | Placement-rader stängs explicit före guard-unit-frågan; även unit-rader stängs före ren verdictberäkning. Orörd. |
| InterpolatePosition / StepHoursDB | Håller inga rader, anropar LoadWorldGraph sekventiellt. Orörda. |
| LoadWorldGraph → LoadTileGraph/LoadRoadSet | Varje reader stängs på funktionsretur innan nästa fråga börjar; inga DB-anrop inne i looparna. Orörda. |
| NearestSea/UnclaimedLand/SettlementNeighbor | Stänger varje reader före nästa iteration/fråga. Orörda. |
| loadSeaHorizons / SweepLiveRadiusAlong | Rader stängs före ren SetSeaHorizons respektive skrivning av minne. Orörda. |
| loadPlacedOrdinals | En reader och skanning, stängd vid retur; inga nästlade frågor. Orörd. |

## Regression och mutation

`TestPlacementLiveEyesTransaction` kör riktiga auth/HTTP POST och därefter riktig PG-TX
för fyra separata fall: marching-unit, outbound, returning, combined. Interpolerade
landögon jämförs som position→antal: enhet(6,0), utgående/återvändande bud(4,0),
med fasta halva tidsfönster. Kombinationsfallet kräver alla tre, så ett fallback-öga
kan inte råka uppfylla en annan enhets förväntan. Pool(MaxConns1)/TX har samma ögon;
efter LoadLiveEyes måste en ny SELECT1 lyckas i samma TX.

De befintliga `TestPlacementConcurrentPOSTsSerialize` och
`TestPlacementSingleConnectionCannotStarve` förblir gröna: slice11:s skydd behålls.
Befintliga courier/passage/callback/siktlinjetester ingår också i hela sviten.

[mutations.py](mutations.py) flyttar själva interpolationskoden tillbaka in i en reader
åt gången: unit, outbound, returning. Alla tre kompilerar och ger namngiven assertion
RED, återställs till exakt SHA/bytes och får GREEN på varsin färsk183.
Inga syntetiska svar, injicerade PG-fel eller tidsfördröjningar ersätter databasen.

## Naturligt användarprov

[native.py](native.py) startar egen PG16/Redis/server med rena miljövariabler och
nyregistrerade konton. Vanlig join→grundning (ren inland och kust), vanlig keryx
unit list→explore, kontrollerar att enheten faktiskt är marching före efterföljande
parallell placering, riktig CLI remove/add och Firefox1280×900/390×844. Ingen SQL-skrivning,
spelar-/svarsoverride, livekonto eller adminåtgärd i scenariot.

Första riggförsöket bad om60dagars expedition, vilket spelet korrekt nekade (tillåtet4–30).
Receptet rättades till30. Nästa försök valde koordinater som saknade hex eller redan
var helt upptäckta, och marschen nekades korrekt; slutreceptet väljer i stället verklig
känd landmark från spelarens vanliga map-svar, längst från staden. Båda fallen finns i
`native-first-attempt.log`/`native-second-attempt.log` som misslyckade riggförsök;
`native-run.log` och `native-final/` är slutprovet. Ingen produktändring för avslaget.

## Leveransbevis

Första kodcommit `a6f12654`, loggtillägg `5828cd43`, bas `ed4e8804`, migration183. Övriga commits är bevis.
Första fullGo exit0 i `full-go.log` (world147,798s), vet exit0 i `vet.log`,654JS pass/0fail/0skip i `js.log`. Slutbevis efter tillägget: `full-go-logging.log`/`vet-logging.log` på5828cd43.
Naturprovets processprovenance finns i `native-final/evidence.json`; råa privata
configar, token och serverbinärer kopieras inte till git.

Begränsning: direkt reproduktion av produktionens exakta500 saknas i den lilla HTTP-fixturen;
den verifierade regressionen är nästlade anslutningsfrågor/tappad interpolation/poolsvält.
Claude granskade och integrerade med denna begränsning som51e805a4. Inga livehandlingar eller GPT-rundor av
Codex före integrationsbeskedet; runda3 startades efter uttryckligt klartecken. Ingen merge/push/deploy av Codex.

## Färsk livekopia: exakt Arete-POST

På Claudes begäran togs en ny **läsande** pg_dump efter runda2, när Arete har en
marscherande spanare. [snapshot_repro.py](snapshot_repro.py) återställer samma privata
arkiv oberoende i två PG16/Redis-instanser. HTTP-server mastered4e8804 respektive
fixa6f12654 använder samma klonade stad/world/player; ingen ändrad speldata-fixtur.
Endast lokal JWT-signatur används för klonens Arete; produktionens kontohemligheter
hämtas inte och inga live-HTTP-spelhandlingar görs. Tickintervall3600 håller körningen
fri från normala snabba privata testticks. Privat baseline har en extra println(err)
vid just could-not-lock-hex; den påverkar inte frågeordningen.

Resultat i [snapshot-repro.json](snapshot-repro.json): **master201 → fix201**, ordinal8
båda. Det ursprungliga500 återkom alltså inte, och inget underliggande hexlås-err
kunde fångas. Detta bevis används inte som ett RED→GREEN-påstående. Dumpens SHA,
klonernas ursprungstillstånd inklusive marching_units, health, request/response och
tidsåtgång redovisas. Dump/serverloggar/binärer hålls privata utanför git.

Snapshotreceptets första två restore-försök träffade Postgres temporära initserver
som höll på att stängas; beredskapskontrollen är rättad till TCP127.0.0.1 före restore.
Slutprovet ovan körde båda armarna färdigt. Inga databas-/serverloggar från produktion
skrevs om för att få fram ett fel. Claude har uttryckligen godtagit överlämning med denna
begränsning eftersom TX-paritet/interpolationsfelet är verkligt.

## Parkerat bifynd

Naturliga inland-provet visar Economy-tabellens `Food after army -0.2 per day` bredvid
status `surplus` (`native-final/inland-goods-firefox.png`). Troligen olika definitioner
(folkets mat respektive efter armén); inte diagnosticerat och ingen ändring i denna slice.
Lämnas till Claude för texttriage, blockerar inte reader-livscykelfixen.

## Claudes tillägg07:44 — alla placeringsfel loggas

25 befintliga500-grenar (20PlaceGubbe,5UnplaceGubbe) använder nu respektive handlers
lokala fail-funktion. Varje sådan skriver `slog.ErrorContext("placement failed")` med
operation, steg, world/settlement/player-ID och det verkliga `err`; `errors.As` ger
även `pg_code` om felet innehåller `*pgconn.PgError`. Klientsvaret förblir samma generiska
500-text. Inga felaktigt lyckade svar eller nya loggar för normala409/404 införs.
Kodcommit `5828cd43` ovanpå den tidigare granskade leveransen.

`TestPlacementInternalErrorLogsDatabaseCause` orsakar avsiktliga riktiga PG-fel med
lokala fixturespecifika INSERT-/DELETE-triggers. Båda HTTP-verben måste ge500, behålla
generiskt klientsvar och skriva ERROR med orsak, steg, rätt ID och SQLSTATEP0001.
Det är ett observabilitetstest, inte en reproduktion av produktionens500.
Tre nya fysiska mutationer tar bort place-orsak, unplace-orsak respektive PG-kod;
namngiven RED→exakta bytes→färsk GREEN i `logging-mutations.log` (totalt sex mutationer).

Den nya slutliga fullsviten är `full-go-logging.log`, exit0/world149,986s, vet `vet-logging.log` exit0 på5828cd43.
Tidigare `full-go.log` gäller a6f före loggtillägget och ersätter inte slutprovet.
JS-koden är byteoförändrad från654pass/0skip-körningen. Naturligt CLI/Firefox-prov påa6f
verifierar den oförändrade lyckade spelvägen; det nya snapshot-provet nedan kör5828cd43.

### Fyra samtidiga HTTP-placeringar på färsk livekopia

[parallel-snapshot-repro.json](parallel-snapshot-repro.json) är den begärda parallella
varianten: samma privata dump som det sekventiella provet, **ny** PG16/Redis-återställning
för varje arm, fyra trådar släppta från samma barriär mot Aretes stone#8.
Mastered4e8804: **201×1/409×3**, fix5828cd43: **201×1/409×3**. Ingen500 i någon arm,
inget hexlås-err att fånga. Båda klonerna har Aretes marscherande spanare före serverstart.
Ingen liveplacering gjord. Detta är ärligt ytterligare ett negativt försök att återge
produktionens exakta500, inte evidens för en500→201-övergång.
