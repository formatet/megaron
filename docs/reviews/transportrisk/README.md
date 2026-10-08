# T1 — transportrisken bor i havet (Claude, 2026-10-08)

Beslut: vault `megaron_transportrisk.md` (Timothy 2026-10-08). Order: `.agents/order-transportrisk.md`.
Gren `claude/transportrisk` från master `84dd1ef5`. Codex inventering (`codex/transportrisk @3f579bd5`) användes som karta.

## Delat i två slicer
- **T1 (denna):** platta tärningen riven; storm per sjöhex för alla skepp; förlis tar skepp, last och inskeppade trupper.
- **T2 (nästa):** löparens öde — räddning när skeppet sänks i strid, förlust i storm, omedelbar dispatch med budets fulla innehåll.
  Fram till T2 gäller passagens befintliga väg för en löpare på ett förlist skepp (förseglad, tillbaka till hamnen, väntar).

## Kontrakt (byggt)
1. `tradeRiskPct`/`tradeLostReasons`/`deliveryLoss`/`isInternalTransfer` och tärningen i `TradeReturnHandler` borta. Gamla `TradeLost` läses som förut.
2. Land: ingen slumpförlust. Interception oförändrad.
3. Hav: `combat.SeaStormScanHandler`, `ScheduledSeaStormScan`, prioritet 5 ("havet", före ankomst 10). Varje sjöhex (`coastal_sea`/`deep_sea`) som skeppet GÅR IN I på sin sparade väg rullar `StormChancePerSeaHex = 0.05`. Starthex och flod rullar aldrig. Träff = −1 skrov. Skrov 0 = förlis: skeppet `disbanded`, inskeppad trupp `disbanded`, transport `foundered`, expeditionsraden raderas (kartkunskap kvar).
4. Utfall slås en gång och lagras: `ShipStormDamaged` / `ShipFoundered` (`SeaStormPayload`, bär `owner_id` = ägarens Wanax). `sea_storm_progress` (mig 161) är resans högvattenmärke i samma TX.
5. Samma risk för alla: flottor, trupptransport, expeditioner, handel, transfer, gåva. Rullningen läser varken ägare eller ärende.
6. Gåva: `GiftLost` reason `foundered`.
7. Ytor: webb (`format.js`, `gifts.js`), keryx (`printSeaStormLine`, `goods`-text), Codex (`sea.md` §Storms, `trade.md`, `transfers.md`, `gifts.md`, index-kinds). Skrov visades redan (war.js, keryx unit).

## Bevis
- Röd baslinje: `red-door.log` — 200 externa leveranser/returer förlorades på `84dd1ef5` (leverans 1, retur 36).
- Full färsk Go: `full-go-raw.log` (24 paket ok, exit 0, migration 161). `vet.log` tom (exit 0). JS: 444 pass (`js.log`).
- Mutationer, alla röda → återställt grönt:
  - tärning kvar vid dörren → `red-door.log` (baslinjen är mutationen)
  - risken beror på ärende (transport hoppas över) → `SameRiskForEveryErrandAndOwner` röd
  - storm utan skrovförlust → två tester röda
  - förlis utan lastförlust (transport kvar `in_transit`) → röd
  - förloppsmärket skrivs inte → omrullning (5 resp. 55 rullningar) röd
- Webbens fixtur `web/static/js/megaron/ui/testdata/ship_foundered.json` är en payload servern verkligen skrev; Go-provet jämför nycklarna mot DB-raden; keryx-provet läser samma fil.

## Antaganden att pröva (TEXT/playtest, inte BILD)
- Flod räknas inte som sjöhex (beslutet säger "hav").
- Storm- och förlisnotisen når ägaren genast, som strids- och rånnotiser redan gör. Om det ska gå fysiskt är det en kanonfråga.
- En resa som redan var till sjöss vid deploy rullar bara hex från dagen före första skanningen (ingen retroaktiv storm).
- Ingen BILD: inget nytt visuellt element, bara notistext.
