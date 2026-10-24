# S1 — skeppet kommer hem innan maten tar slut

Kontrakt: preventiv återfärd via befintlig fysisk retur när skeppets mat bara räcker hem med en ticks marginal; undantag för ett nåbart aktuellt stadsmål. Alla mottagande städer ger mat upp till hembehovet, efter egen dagsmat, utan silver eller ägarbesked. Nytt besked når ägaren först vid hemkomst. Historiska events behåller semantik.

Bas: f76eba5e. Gate: bevisar fysisk sjöresa i kedjan. Scope: combat/economy/transport/messenger-ankomstsubstrat, klockinjektion och fyra notisytor. Non-scope: nya frakt-/lastregler, förlisbesked, ljud/BILD och balans.

Claude12:54 godkände även sen svältfallback utan omedelbar notify; audit-event kvar. Sjunkande skepps befintliga meddelande lämnas orört. Fraktens fysiska avbrott är uttryckligen frågat före förändring eftersom freighting binds till transportjourney/carrier witness.

Bevisplan: named gammalkodsRED, återfärd med besättning intakt, främmande stad och tom/egenransonsvärd, idempotent replay, berörda färska paket, fysisk source-mutant och klientnotis/Codex-paritet. FullGo först S5 enligt order. Inga prodskrivningar.

## WIP checkpoint — STOPP från Claude12:57

Inte färdigleverans och inte mergeklar. S2–S5 har INTE startats.

Sparat: clock.Clock i UpkeepHandler och alla konstruktörsanrop, preventiv retur med sparad fysisk position/hemrutt, food_return-hemkomstkind, borttagen omedelbar notis för nya sena fallbackreturer (audit kvar enligt Claude), economy.HospitalityFood samt wiring på combat och transport/trade/gift-ankomster. Webb/Keryx/Codex har hemkomsttext, historisk UnitReturnedStarving-läsning bevarad.

Godkänd avgränsning12:55: freighting avbryts INTE på matbrist i S1; nåbart stadsmål fortsätter och får gästfrihet vid ankomst. Frakt som inte når målet med maten är uppföljning, ingen ny lastregel. Frekvens live är ännu INTE mätt.

Bevis hittills: TestReturnFoodLeavesBeforeCrewStarves gammalkodsRED (20crew står still trots hemreserv). KlientbaselineJS/CLI RED→GREEN; fysiska klienttextmutanter RED→byteexakt restore. FullJS749GREEN före sista ikon/dokumenträttelse; riktade formatter/Codex16GREEN efter. Riktade CLI-notisprov GREEN.

Senaste serverförsöket stoppade i compile på saknad economy-import i transport/arrival.go under samtidig wiring. Importen är nu tillagd, men INGEN efterföljande kompilering/test påstås. Raw loggar finns i tests/.

Återstår: kompilera slutbytes; färska combat/economy/transport/handlers-paket; fler DB/lifecycleprov på gästfrihet/tomvärd/egenransonsreserv/cargo/sparad rutt/nåbart stadsmål/replay/hemkomstnotis; uppdatera gamla fallbacktesters intent/notisförväntningar; egen servermutation; granskning av frakt-homejourney och giftwiring; slutlig klientparitet och proof. Inventera och dokumentera dagens förlisbesked utan att ändra det. Ingen fullGo före S5 enligt ursprungsorder, men fortsatt arbete kräver ny startorder.
