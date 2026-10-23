# Proviantens heltalsgräns

## Slice-kontrakt

Problem: 0.7 mat / 0.1 per tick visas som 6 hela dagar, fast sju ransoner finns.
Spelarsanning: mätaren visar hela täckta dagar vid decimalgränsen.
Invariant: ingen ändring av ranson, lastning, betalning, förbrukning eller eventsemantik; verklig brist rundas ned.
Scope: combat.ProvisionDaysLeft och dess API-konsument, riktade regressioner.
Non-scope: stadsmat, lagermodell, nya tidsbegrepp och ekonomisk balans.
Acceptans: 0.7/0.1=7; verklig brist=6; sista delranson/tomt fortfarande olika; server och klienter använder samma svar.
Stopp: ändringen kräver ändrad förbrukning eller spelregel.
Bevisplan: namngivet gammalkods-RED, regressioner för avrundning/brist samt autentiserad HTTP; egen källmutation; fresh full Go, JS och vet en gång; deploy med healthz och grundkontroll.

Huvudgrind: bevisar sjölogistikens läsbara matmarginal.

## Resultat och verifiering

`ProvisionDaysLeft` korrigerar en enda representerbar flyttalsposition före golvdivisionen. Ingen fast mängd mat eller allmän avrundning uppåt tillkommer. Matkällan, ledgern och konsumtionen är orörda. Webb och Keryx läser samma `provision_days`; inga lokala ransonsformler eller artikelregler behöver ändras.

- Namngivet gammalkods-RED på .7/.1 och .3/.1 samt verklig autentiserad GET /units med fraktande skepp: `baseline-red.log`.
- Färsk migration205, riktade combat/API/transport-provianttester gröna: `targeted-green.log`. HTTP testar .7→7, verklig brist→6, delranson→0+aboard och tomt→0+false; varumanifestet är bevarat.
- Fysisk mutation till Ceil ger namngiven verklig-brist/delranson-RED, byteexakt återställning: `mutation-ceil-red.log`. Återställda källor därefter i fullsviten.
- EN hel Go-svit mot färsk tom DB/migration205 grön, inklusive world155.490s: `full-fresh.log`. Vet grönt: `vet.log`. 738 JS gröna/0 fail: `js.log`; inkluderar riktig webbkortsrendereing och klientens serverfält. Keryx paritetsprov ingår i hela Go-sviten.
- Ingen visuell förändring utöver korrekt tal; ingen ny BILD-gate. Ingen ändring av migrationsversion eller gamla events.
- Live före: process67d0b15/mig205/ok; DB205 dirty=f,109sjöenheter,1värld. `live-before.log`, `live-ground-before.log`.

Begränsning: fixen gäller slutdivisionens enstaka flyttalssteg. Verklig brist och fel i lastning/förbrukning kompenseras inte av mätaren. Ingen ändring av ekonomisk balans.
