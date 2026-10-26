# Redirect under expeditionens hemresa

Kontrakt2026-10-26 före kod. Timothy: ”varför tror spelet att5thspearmen står på4924?” följt av ”ja—hitta buggen!”.
Problem: ExecuteRecall redirect raderar expeditionsraden men behåller march_intent=explore_return och home_settlement_id. Ankomsthandlerns gamla returngren gör sedan förbandet till garnison i hemstaden trots att det fysiskt har gått till en annan hex.
Spelarsanning: en redirect avbryter expeditionen och blir vanlig fysisk marsch till det valda målet; tomhex ger positioned, egenstad bara garnison om förbandet nått dess faktiska hex.
Invariant: ingen teleport, ingen retroaktiv omtolkning av events; inga garnisoner på avstånd. Felaktig befintlig landgarnison rättas till positioned på sin sparade fysiska hex, med oförändrad support/payor.
Scope: ExecuteRecall:s intent/home-livscykel vid redirect av explore_return; verkligt redirect→arrival-prov; inventering/backfill av landgarrison som inte står på sin stad; bevis/datarevision.
Non-scope: utåtgående explore-intent/legacy-pointexpedition ändras inte. Budförföljningsslicen är separat sparad WIP5f6aa1f3. Ingen ändring av verklig recall-home eller skeppens havshex/garnison; ingen flytt av fysisk position/hemstad/underhåll.
Acceptans: redirect släcker explorationsintent+hemankare; tomhex slutarpositioned, hemmahex korrektgarnison; gamla fördröjdaarrivaljobs no-op; recall behåller expedition/report; korrigering bevarar alla fysiska koordinater/gods/ägare/support.
Bevis: liveSQL+events5th, rött→grönt livscykelprov, migrering216→217/backfillkontroll, mutanter/exaktrestore, helfärskGo/JS/vet, isoleradHTTP/worker/Keryx och livehash/datajämförelse.
Gate: blocks korrekt läsning och fysisk ordergivning i geografisteget.

## Resultat

Kärna `1feeec6d`. Endast redirect under `explore_return` släcker hemresans intent/hemankare; utåtgående legacy-explore samt vanlig land/navalrecall är oförändrade. Ingen ändring av historiska eventpayloads/handlers behövs för nya korrekta ben. Inventeringen fann noll pågående felaktiga returben och exakt en redan felmärkt landgarnison:5thSpearmen på(49,24), Kommos på(54,32).

217 korrigerar den omöjliga relationen till positioned/settlement=NULL. Sjöhamnars garnisoner på angränsande havshex är undantagna. Ett nytt auditutfall dokumenterar korrigeringen; gamla händelser förblir oförändrade. Down återinför avsiktligt inte den felaktiga relationen.

Grindar: hel färskGo26grön på1feeec6d (world155.245s),794JS/vet/fyra dayguards gröna; fyra assertion-röda mutanter + exaktrestore; privat driftkopia216→217→216→217 med endast denna korrigering och bevarade fysik-/payor-/resurs-/kontohashar. Verklig isolerad Keryx→HTTP202→Runner→worker→fältankomst samt senare gammaltreturjobb gröna. [Täckning/exakta mutanter](coverage-and-mutations.md), [offlineuppgradering](upgrade-proof.json), [nativebilder](index.html).

BILDtestets första desktopbild togs mitt i draweranimationen; slutriggen väntar på öppningsövergången och alla slutbilder läses vid1:1. Scenen har uttrycklig inkommande returrutt; den utger sig inte för att vara Timothys levande värld. Ingen nyUI eller spelhandling tillkommer; befintlig webb/Keryx/Codex använder den korrigerade serverplatsen/statusen.

Drift: LIVE `0b31b00f`/migration217ren. [Deploybevis](deploy-proof.json) visar5th positioned på(49,24),settlement=NULL, samma ägare/support och exakt1correctionaudit; värld15spelare/5städer/18enheter/85varurader bevarad. Nyprocess bevisades via health före schemaläsning; inga nya felloggar. Budförföljning är separat WIPb907d1b0, återupptas efter denna akuta fix.
