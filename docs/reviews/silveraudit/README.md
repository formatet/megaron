# Komplett silverrevision

Kontrakt före kod: världens ägda och fysiskt reserverade silver måste räknas
en gång, oavsett om det finns i stad, host, kolonipung, offerescrow eller last.
SilverAuditV2 använder verkliga realiseringskrediter för gruvproduktion;
SilverAudit V1 läses och emitteras med sin gamla innebörd. Scope: intern
revision, realiseringsinstrumentering/migration, analysverktyg och prov.
Ingen ny spelarnotis eller publik förmögenhetslista (FOW oförändrad).
Acceptans: en verklig bevarandesekvens, carried-mutation rött/byte-grönt,
färsk Go/vet, kompatibilitet och migration, merge/deploy/healthz.
Kanonkonflikt parkeras i BESLUT; andra köarbeten fortsätter.

Baslinje c7b9abad/mig178: färsk TestUpkeepSilverBookkeeping grön.

Inventering: combat/silver_audit.go V1 räknar bara aktiva ägda städer +
pending BUY, och uppskattar mined som aktuell rate×elapsed. Tillkommande
fysiska poster: active founder_phase (founded kopia ej pengar), levande
units.carried_silver (disbanded förlorad), transport_goods på in_transit
transport (levererad/intercepted/lost kopia ej pengar), pending SELL med
silver som själva varan, samt accepterad SELL:s reserverade silverretur
i scheduled_events.then_return medan första transporten är in_transit.
Efter leverans ligger samma pengar i returmanifestet och reservationen
ska därför inte räknas igen. Neutral stadsstock redovisas som ägarlös,
medan sunk/collapsed städer är förlorade/inaktiva liksom V1.

## Resultat och grindar

Produktion 16e7579e (mig179). Egen kärndiffgranskning: V1-filen och gamla
payloadläsare lämnas oförändrade; V2 har separat epoch/baslinje och atomisk
claim/event/realiseringsräknare. Ingen publik finansinformation tillkommer.
Räknaren ligger per stad, behålls vid stadsförlust och uppdateras i samma TX
som silverraden. Revisionen använder en sammanhängande databasögonblicksbild.

| Grind | Bevis |
|---|---|
| Kod | go-full.log: full färsk PG16/mig179 Go grön (world91,267s); vet.log grön |
| Bevarande | TestSilverAuditV2Conservation: 0→host480→stad730→gruva735 (faktiskt5)→fullt lager0 utvinning→sold733→gruva735 (faktiskt2)→BUY-reservation50→trade-manifest50→verklig leverans→transfer25/leverans→carried15→verklig koloni15. Varje steg: delta = krediterad utvinning + nya fasta endowments − faktisk sold; flyttar har delta0. Genesis/escrow är explicita finansiella fixturer; payroll, dispatch, trade/transfer-delivery och koloniväg använder verkliga domänfunktioner. B:s native förstagrundning verifierar dessutom HTTP-genesisvägen. |
| Fullständig stock | TestSilverAuditV2AllBucketsAndDeferredHandoff: alla sju poster samtidigt288, varav neutral8 och ägda280; disbanded/delivered/accepted kopior ignoreras. Verklig SELL-leverans flyttar deferred70 till returnmanifest, även innan worker markerar processed_at, utan dubbelräkning. |
| Fel/migration | AppendFailureCanRetry: fel vid sista eventskrivningen rullar tillbaka stock90/calc0, credit0 och claim0; retry ger precis1 revision och10 credit. RollbackAndRateChange: ingen gruvcredit från rollback, gammal cap-klippt rate10 + ny rate4 =14. Populerad179-down/up bevarar735 och gamla V1-fält; ny epoch öppnar med mined/delta0. |
| Mutation | mutate.py: carried-missing och mined-rate-estimate, fysiskt named RED → byte-identisk återställning → separata färska GREEN; fyra loggar |
| Analysverktyg | reader_proof.py: egen färsk mig179/dirtyf, verklig Go-V2 JSON och read-only DB, alla7 Pythonreader-prov utan skip, faktisk alfa_economy-rapport i actual-economy-report.md. Öppningsrevision och epoch, sju stockposter, total/median/max/nollandel samt övrigt netto visas separat från V1. |

Repro: `python3 docs/reviews/silveraudit/mutate.py`,
`python3 docs/reviews/silveraudit/reader_proof.py`, `tools/gotest.sh`,
`go -C server vet ./...`. live_proof.py tar strikt read-only före/efter-
provenance, gamla eventpayloadhashar och founderstock vid integration.
Ingen BILD eller nytt spelarverb: denna slice är intern mätning och analysverktyg.

## Kända avgränsningar

V2 mäter tillgängligt silver; förlorade stads-/unit-/transportkopior räknas inte.
Aktiva neutrala stadsvaror ingår och redovisas som ägarlösa. All världens
registrerade Wanax och tillgångsägare räknas i fördelningen, inklusive AI och
ägarlösa/dispossessed Wanax med0; global ägarlös stock läggs inte i medianen.
Övrigt netto är kranar minus sänkor, inte ett påstående om fel. AI-guvernörens
redan kända tysta10-avdrag finns kvar som separat VERIFIERING, ingen beteende-
ändring i denna slice. Realiseringsinstrumenteringen förutsätter den befintliga
skrivregeln: silverrealisering omankrar calc_tick; kvarvarande lat produktion
realiseras uttryckligt i revisions-TX. Gamla kroppar som inte följer den regeln
ska inte räddas genom en gissad rate×elapsed-credit.

Riggens första psql-prov fann att lokal klient saknas; reader_proof använder
nu den egna PG-containerns klient. Ett test frågade först efter processed i
stället för processed_at; själva handoff-invarianten var redan grön. Inga skips
eller påstådda rena baselinefel döljer dessa riggfynd. Ingen kanonblockerare.
Integration/deploy verifieras separat.

