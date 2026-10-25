# Kompletterande DB-reproduktion — STOPP före fix

Bas 7e478f88, produktionskod oförändrad. Den parallella trådens rapport och JS-replay
är committade i af3c1d75; detta tillägg äger bara Go-provet, dess fullständiga logg
och ett reproducerbart read-only SQL-underlag.

```sh
tools/gotest.sh ./internal/combat/ -run '^TestShipReport43640c42_DeparturePhase$' -v
```

Körningen på en tom databas, migration 212, är **avsiktligt RÖD**:

| Fas | Initial position enligt sparad rutt | Annonserad ETA / tickgräns | Resultat |
|---|---|---|---|
| Precis tickgräns 103 | Första inträdeshexen (59,27) | 20:10:30 / 20:10:30 | GRÖNT |
| Rapportens fas, +44 s | Målhexen (58,26) direkt vid StartMarch | 20:11:14 / 20:10:30 | Två RÖDA assertioner |

Testet använder riktig `StartMarch`, sparar rutten via dess oförändrade
produktionsväg och frågar samma `RoutePositionAt` + `tick.Anchor.MilliAt` som
serverns enhetsyta. Ruttens tre steg kostar 200/200/200 milli-ticks och ger en
avrundad varaktighet på en tick. Val A:s första inträdeshex vid avgång respekteras;
provet kräver **inte** att skeppet står kvar i ursprungshexen.

Roten är olika tidsursprung: heltals-start_tick backdaterar den fysiska rutten
till tickens början, medan `arrives_at` räknas från den faktiska ordertiden.
Den historiska rutten är överskriven; testets lagliga kortaste rutt är en
rekonstruktion, inte ett återfunnet historiskt JSON-värde. Ingen omedelbar
serververkställning påstås: det sparade jobbet kördes cirka 31 s efter ordern.

Live-extraktionen kan upprepas med `tools/ship_immediate_readonly_20261025.sql`
via `psql -X -qAt -v ON_ERROR_STOP=1` i det auktoriserade prod-lässkalet.
Varje frågeblock är inramat i `BEGIN READ ONLY`; ingen prod-skrivning behövs.
Ingen fullsvit eller mutation kördes: uppdraget är utredning och rött bevis,
inte implementation. Inga server-/webb-/Keryx-/Codex-produktionsfiler har ändrats,
och Varvet 7ad899ac är orörd. Claude beslutar tidskontraktet och nästa fixorder.
