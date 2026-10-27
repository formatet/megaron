# Kharis som tal

## Sammanfattning

Kult visade sinnelag utan tal och aktuell följd; profil och Keryx saknade delvis betydelsen. Servern beskriver nu kharis, band och aktuell chans på samma underlag som ticklogiken. Webb, Keryx, nya notiser och rivningsbesked använder denna beskrivning. Codex förklarar skalan, banden, taket, slumpen och omen.

Kod `0f645cab` (kärna `0d1853c3`), bas `92cb709a`. Slutverifiering GRÖN: hel färsk Go (world157.208s),804JS,vet,fyra tidsgrindar,sex fysiska mutanter med exakt återställning och färsk grön kontroll,åtta Firefoxbilder från två rena1280/390-körningar. LIVE `00180a07`; processhash verifierad, migration217ren, samma värld15spelare/6städer/18enheter/102varurader och8byte-exakta ändrade statiska filer. Se `deploy-proof.json`. Ingen migration eller balansändring.

## Slice-kontrakt

Order `.agents/order-kharis-tal-20261025.md`. **Bevisar** kedjegrinden: Kult, egen profil och böne-/rivningsytor ger spelaren begriplig aktuell relation till gudarna. Chans för spontan välsignelse är skild från bönernas utfall. Tal och trösklar kommer från serverägaren. Gamla notiser ändras inte.

## Baslinje och experiment

- Kult hade bara mood; egen profil hade ett naket tal. Straffregeln fanns separat; välsignelsechansen var privat konstant i ticklogiken.
- `BlessingProbability` delas nu av tick och `DescribeStanding`. Tickens tidigare kortslutning och slumpordning bevaras. Straff använder befintliga `PunishmentProbability` samt den bestående skyddstiden.
- Ett stående tempel räcker för dagens välsignelseroll; ingen ny offer-/bemanningsgrind införs. Behållen kharis över sänkt tak gäller fortfarande.
- Första breda körningen fångade gammalt exakt femfältskontrakt i profiltestet. Nästa breda körning fångade mitt nya 500-krav när gamla fixturer saknade kharispost. Det kravet är borttaget: ny beskrivning är otillgänglig utan underlag, befintliga läsningar fungerar. Detta var underkända körningar, inte gröna baslinjer.
- Avrundning till en decimal krävde uppdaterade presentationsförväntningar. Formeln räknas aldrig om i klienten. Förklaringen anger att det är realmens förlust före tempelproduktion.

## Grindar och reproduktion

- `tools/gotest.sh`: full färsk DB, migration217. Slutlogg `full-go.log`; berört API-paket separat i `api.log`.
- Hela JS-sviten: `js.log`. Vet: `vet.log`. Fyra tidsvokabulärgrindar gröna.
- Sex fysiska servermutanter: `mutations/proof.json` listar exakt ersättning och förväntat test, med separata assertion-röda loggar. Byte-identisk återställning och ny färsk grön kontroll krävs av verktyget.
- `python3 tools/kharis_tal_bild_20261027.py OUTPUT`: ny egen PG/Redis/server, verklig register/join/found, explicit tempel2 och kharis4/70. Riktiga API/profil/Keryx status/me/rite, Firefox1280/390 två gånger. Inga utbytta HTTP-svar eller DOM-fixturer. Provenance från processen i `native/proof.json`; produktionshashar i `native/source-before.json`.
- BILD: [A1–B2](index.html), alla åtta skärmbilder under `native/`. Tal och aktuell chans ryms utan horisontell scroll. Mänsklig textbedömning återstår i spel.
- Drift: `ground-before.json` och `deploy-proof.json`. Processhash, migration217ren, världens identitet och grundantal samt ändrade statiska bytes jämförs skrivskyddat.

## Täckning och avgränsningar

| Yta/ägare | Bevis | Inte täckt av nya prov |
|---|---|---|
| Gemensam standing | 4/5/14.9/15/30/59.9/60/90; tempel på/av; skydd; exakta band/chans; mutant | Ingen frekvensmätning av slump över många ticks |
| Province API | sex verkliga DB-fall; numerisk paritet mot tickfunktionerna; privat fält filtrerat; mutant | Ingen ny svit över alla äldre ofullständiga DB-tillstånd |
| Egen profil | eget/världsisolerat underlag, serverbeskrivning och mutant; riktig Keryx me | Ingen publik profiländring |
| Rivning | befintlig faktiska skuld/floor/idempotensserie utökad med efterföljande standing; mutant | Ingen ny native rivning i browserriggen |
| Notiser | bestående snapshot i nya band-/förfallsnotiser; bandlivscykelprov och mutant | Äldre notiser får ingen retroaktiv chans; ingen ny native notisklickserie |
| Kult/böner | servertext vid standing och böneavsnitt, verklig native panel och Keryx rite-lista | Ingen ny bön eller ändrad böneframgång; inget nytt native offerköp |
| Klienter | hela JS/Keryx-sviter samt verkliga API→Keryx och API→Firefox-läsningar | Sex mutanter avser serverns kärna, inte varje presentationsfil |

## Metodisk lärdom

Ny läsmodell ska markera saknat underlag utan att skärpa andra etablerade endpointkontrakt. En chansbeskrivning måste skilja grundrisk från bestående skydd och spontan välsignelse från bönesvar.
