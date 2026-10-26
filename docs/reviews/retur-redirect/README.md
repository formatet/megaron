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
