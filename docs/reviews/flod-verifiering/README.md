# Flodvyn — verifiering 2026-10-23

Flodens karta, panel och Keryx har samma namn utan att avslöja namnet i dimman. Riktiga server-/klientflöden, 24 webbläsarmätningar och 0 fynd i Firefox/Chromium/WebKit på 1280 och 390 px. Ingen spelkod ändrad.

Provenance ur `/healthz`: `{"commit":"a05cddc+dirty","migration":205,"status":"ok"}`. Keryx byggd från master 99525dba. Serverns dirty-markering är den befintliga acceptansprocessens; detta är inte ett påstående om en ren ny produktionsbuild.

## Kontrakt

Problem: aktuell flodvy saknade färskt verkligt browser/API/Keryx-bevis. Spelarsanning: en känd flod kan öppnas och dess namn skiljs från terrängtyp. Invariant: inga geografiska namn i dimman. Scope: verifieringsverktyg och bevis. Inte scope: ändrad terräng, mekanik, rutt, rörelsekostnad eller naturlig upptäckt. Stoppvillkor: befintligt geografiskt namn/anspråk får aldrig skrivas över. Bevisplan: normal registrering/anslutning, explicit egen sikt/anspråksfixtur, offentlig namn-API, riktig karta och CLI, städning i finally; färsk DB för FOW-provet.

## Metod och begränsningar

De befintliga BrowserCity23/BrowserHost23-kontona kände ingen flod. Äldre konton Eirene/ParityNomad/Agamemnon fanns inte i denna rigg. Världen har flodterräng men saknade namngivna floder. Det är en riggförutsättning, inget konstaterat spelfel.

Dedikerat konto RiverProof23 registrerades/anslöts normalt. En egen placerad spearman vid ett befintligt vadställe och ett explicit första-ankomstanspråk sattes med SQL för att isolera layout/FOW från restid. Namnet sattes sedan genom det riktiga, behörighetskontrollerade API:t. Detta bevisar **inte** naturlig upptäckt, fysisk första ankomst, flodfärd, vadning eller varuleverans.

`tools/river_view_proof.py` återger hela provet, vägrar skriva över befintligt anspråk och tar bort sin spearman och sitt anspråk i finally. Det egna kontot och dess normala grundarvärd finns kvar. Inga andra konton ändrades, inget reset/reseed och tick-kadensen är oförändrad.

## Resultat

- Verklig karta: fem kända flodhexar med samma namn; inga namn/place_name i fog.
- Browser: namnet i paneltiteln, generisk terrängrad; kartan och panelen via native klick/tap. Host/Details ingår som regressionskontroll.
- Keryx: faktisk `map --radius 100 --json` och `places` innehåller samma namn.
- Riktat färsk-DB-prov `TestMapRiverNamesFOW` grönt på migration 205 (live/remembered/fog). Rå handlerpayload i `fresh-fow/map-fow.json`.
- Keryx River/NamedRiver-prov gröna; fem inspect_simple JS-prov gröna. Syntax och diffcheck gröna.
- Bilder inspekterade i faktisk skala: Firefox mobilpanel och desktopkarta.
- Ingen full Go-svit eller produktionsdeploy behövs för denna verifieringsslice.

Första körningen använde separat setup-script; en felaktig `/healthz`-URL i det scriptet gav 404 efter skapad siktfixtur. Den sparade enheten återanvändes, ingen duplicerades. Slutverktyget innehåller setup och korrekt health-anrop; slutkörningen kördes separat: samma 24 mätningar/0 fynd, faktisk CLI grön, egen fixtur städad. Ett försök att köra browserkontrollen före namngivning avbröts korrekt med “no known named river”; det var ingen röd produktionsbaslinje.

Kvar i todo: verklig vadning/sjöfärd och flodhandel. Den breda raden “floder otestade” får inte strykas utifrån enbart detta vyprov.
