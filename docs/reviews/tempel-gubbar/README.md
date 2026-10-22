# ORDER 13 — tempel med verklig bemanning

Gren `codex/tempel-gubbar`, worktree `/home/tk/wt/codex-tempel-gubbar`, bas `ea97b332`.
Inventering `03945565`, torrmanifest **före migrationskod** `f7bfd91a`, implementation `b9213c30`, kommentarrättning `141031eb`, Keryx-hänvisningar `df4aaa9d`, regressionstest `913bfe83`.

**BILD före merge:** [bildsidan](index.html). Firefox, 390 och 1000 × 844, has_touch och riktiga `tap()`. Claude tar BILD till Timothy. Inget mergat/pushat/deployat; ingen livedata skriven.

## Slutligt kontrakt

- Economy är ensam ägare av fysisk tempelkapacitet och räknad bemanning: `TempleWorkplaces`/`LoadTempleStaffing`/`RealmTempleStaffing`.
- Två platser per nivå, faktiska `settlement_placement`-rader (`building/temple/cult`). Samma ordinals/pool som mat och andra jobb. Ingen andel, inget additivt arbete.
- `KharisPerTempleWorker = 1.05`, Timothy via Claude 18:17. Full L1 ger 2,1 före befintlig offerfaktor, knapphet och börda. Tick och prognos läser samma bemanningsägare.
- Tempeloptionen är `staffing_only`, med noll varuproduktion och verkligt nivåtak. Ingen dummy production_rule, inget cult-lager. `goods.cult` är kvar som viktlös FK-ankare; fraktreglerna är oförändrade.
- Befintlig beskärning prioriterar mat före tempel före andra jobb. Rivningens befintliga byggnadsrensning tar även tempelplaceringar. Inget gratis tempelgolv vid grundning.
- Offer, rites och deras grindar lämnas oförändrade. Även en obemannad byggnad följer befintliga offerregler.
- Gamla `LaborAllocated` och andra historiska events ändras inte. Nya kharisutfall använder fältet `temple_workers`, inte omtolkad `devotion_sum`; maintenance har `rule_version=temple_workers_v2`.

## Fyra ytor

Temenos: placement-API för tempel, bemanning i province-GET och gemensam tick/prognos. PUT labor och LaborAlloc bort.
Keryx: `staff temple +1` / `staff temple -1`, city och roster visar verkliga assignment; `allocate` bort och status-hänvisningar pekar på staff.
Webb: Citys tempelarbetsplatser har vanliga +1/−1/Fill och platspippar; procentreglaget och dess dolda placeholder bort. Kult visar lokal bemanning samt serverns realm-netto och varning om obemannade platser.
Codex: temples/kharis/citizens/keryx samstämmiga. CLAUDE.md och vaultens tempel/cult-plan/verblista uppdaterade. Umami använder befintliga placement/staff/unplace-events; cult är tillåten roll i dessa, inte fraktvara. Labor-eventrouten och dess aktuella verbtabellsrad bort.

## Övergång och före deploy

Migration 201 (ursprungligen 199, omnumrerad 2026-10-22 efter att live gått till 200) skapar en övergångsmarkör. **Innan workers, HTTP eller tick startas** gör serverns uppstart en transaktion: lås markören, beräkna mål, slutreglera föregående produktion, flytta riktiga jobb, räkna om produktion, droppa `settlement_labor`, markera klar och commit. Fel ger rollback av hela övergången och stoppar uppstart. En andra uppstart är no-op. Schemaåterställning efter genomförd övergång kräver backup; down-migrationen vägrar en förlustfull låtsasåterställning.

Mål för en stad med tempel och positiv gammal vikt: `min(2×nivå, population/100, max(1, round(vikt×population/100)))`. Lediga ordinals i stigande ordning först, sedan icke-matjobb enligt befintlig pruningprioritet (bronze/oil/wine först), högsta ordinal först inom prioritet. Mat flyttas aldrig. Om fria + icke-mat inte räcker avbryts hela transaktionen.

Read-only snapshot 16:14:19 UTC: 8 L1-tempel, mål 16 personer, 10 lediga och 6 omplaceringar i tre fulla städer. Exakt lista i [torrmanifestet](migration-manifest.md), underlag i `migration-dry-run.json`. **Detta är ingen reservation av dagens jobb. Claude kör ny SELECT direkt före deploy**, jämför manifestet och stoppar vid oväntade förflyttningar/brist:

```sh
ssh root@10.0.1.92 'sudo -u postgres psql -X -v ON_ERROR_STOP=1 -d poleia' \
  < docs/reviews/tempel-gubbar/migration-dry-run.sql
```

SQL-filen omsluter endast SELECT i `BEGIN READ ONLY`; databasnamnet följer `tools/alfa_db.py`. Inga UPDATE/DDL mot live har körts av Codex. Migrationsproven kördes på engångs-Postgres; deras DROP TABLE rullas tillbaka i testtransaktionen.

## Funktion och prov

- **Namngiven bas-RED:** `TestTempleHasTwoRealWorkplacesPerLevel` gav gammal L1-cap 0, förväntat 2 (`baseline-red.log`).
- HTTP: L1/L2/L3 cap 2/4/6, första överkapacitetsförsöket 409, samma idle-pool, borttagning frigör personen och sänker faktisk bemanning, ingen cult-stock. Matjobb och tempel kan inte dubbelbemanna en person.
- Övergång: ledig ordinal 5 före omflytt av wine ordinal 4, grain orörd, legacytabell borta, replay no-op; endast matjobb innebär avslag.
- Kharis: 0→1→2→1→0 riktiga workers ändrar projektionen med exakt 1,05 per person. Befintliga faktiska maintenance-/offer-/180tick-prov läser nu explicit placerad bemanning.
- **Egen fysisk mutant:** i separat detached worktree ändrades kapaciteten från 2 till 3 per nivå. Det riktiga HTTP-provet blev RED på alla tre nivåer: tredje/femte/sjunde personen accepterades. Exakt återställning gav färsk GREEN; SHA256 och rena gitstatus verifierades. Se `mutant-red.log`, `mutant-restored-green.log`, `mutation-restore-sha256.txt`. Arbetsgrenen muterades aldrig.
- **Slutlig färsk full Go, migration199 / GOTEST_PORT=57900: GREEN**, inklusive world149,461s (`full-go.log`). **712 JS GREEN**, vet GREEN; Keryx helsvit efter sista texträttningen GREEN (`keryx-final.log`). Den första fullrundan gav ett fel i mitt nya projektionsprov: dess fixture saknade offermat, vilket korrekt gav gain 0. Testet fick explicit grain=100; ingen produktionskod ändrades för felet. `full-go-fixture-red.log` bevaras. Efter korrigeringen kördes slutlig färsk fullrunda.

## BILD och reproduktion

`python docs/reviews/tempel-gubbar/browser_check.py` kör riktiga map.html/CSS/ES-moduler i Firefox med uttryckliga lokala HTTP-fixtures. Det är UI-bevis, medan riktiga DB/HTTP-prov bevisar serverreglerna. Inga inloggningar/liveåtgärder görs. Basen hämtas från `git show ea97b332` med `--baseline`.

Varje bredd körs i två rena browserkontexter. Scener: L1 tom, en worker, två/full, en frigjord, L2 fyra platser, arbetskraftsbrist och Kult med lokal bemanning/netto/offer. Den tredje L1-skrivningen vägras även vid direkt anrop; knappen är samtidigt disabled. `browser.json` redovisar pixel-SHA256 per körning och scen. Samtliga par är lika. `baseline-browser.json` redovisar motsvarande basbilder.

Fixtures låser tid, data, kamera, DPR och canvasuppvärmning; väntar på innehåll, nätverk och fonter och avslutar ändliga CSS-animationer. Den valda arbetsraden centreras med explicit instant-scroll. En mellanliggande körning efter att offer-fixturen kompletterats gav en engångsskillnad i L2-bilden; nästa diagnostiska körning kunde inte reproducera den (pixel-diff tom). Ingen orsak hävdas för den observationen. Slutkörningen använder uttrycklig bildposition och jämför båda rena körningarna strikt; den tidigare osäkra körningen används inte som bevis.

Bilderna har granskats i originalstorlek av Codex för synlig arbetsrad, pips, fullspärr och Kult-bemanning. Slutlig BILD-bedömning återstår hos Timothy. Basbildens tomma tempelalternativ visar den gamla begränsningen; bilderna är inte data från live.

Alla slutliga artefakter har fil-SHA256 i `SHA256SUMS` (inklusive rapport, HTML, browser-script, bilder och loggar).
