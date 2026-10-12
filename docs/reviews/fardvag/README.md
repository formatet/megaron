# Färdvägsstreck

Inventering före implementation på oförändrad **746398ed**. Gren `codex/fardvag`, eget worktree `/home/tk/wt/codex-fardvag`. Claude integrerar; inga merge/push/deploy görs av Codex. BILD före merge.

## Slice-kontrakt

| Fält | Kontrakt |
| --- | --- |
| Problem | Kartan visar gående aktörer utan en alltid synlig återstående färdväg eller främmandes riktningspil. |
| Spelarsanning | Mina aktörer visar vägen framåt från aktuell position; främmande visar endast aktuell riktning. |
| Invariant | Inget bakom aktören, ingen främmande rutt/mål/ETA, inga streck/pilar utanför LIVE-sikt. |
| Scope | Serverns befintliga kartprojektioner där inventeringen visar saknade fält; delad webbrendering, keryx sightings, Codex, prov/rigg. |
| Non-scope | Rörelsebalans/pathfind, nya diplomati-/krigsregler, ny budruttlagring, kingdom/oikos. |
| Acceptans | Progress0/mitt/1 och LIVE-klippning; säker foreignpayload+mutation; fyra ytor; färsk Go om server rörs/JS/vet; Firefox1280+390 med fyra egna slag och främmande armé/skepp. |
| Stopvillkor | Ny relation eller ändrad informationsrätt behöver kanon; recipient-transporter särskilt avstäms före DTO-ändring. |
| Bevisplan | Baslinje, verkliga API-payloads, rutt-/privacyprov och fysisk mutation, aktuellt processbygge, BILD1280+390. |

Kedjegrind: bevisar kartans geografiska läsning och orderriktning, utan ny brons-/elitmekanik.

## Inventering före kod

| Slag/relation | Befintlig payload och positionsägare | Lucka/minsta ändring |
| --- | --- | --- |
| Egen armé/Host | `/units`, owner-only `ListUnits` (world.go2016). `attachUnitPaths`2108 skickar frusen `LoadActiveRoute`-path; `unitSummary`2181 bär ISOtider och arrival_tick/duration_ticks. `current_q/r`2726 kommer från sparad rutt. | Step costs saknas för vägvisning i samma terrängtakt; lämna NULL/legacy ärligt utan påhittade kostnader. Kartans unitPixelNow interpolerar idag jämnt. |
| Eget skepp | Samma `/units`; category naval, sparad marschrutt. Fraktens sjöresa bor i transports och `/trades`. | Samma routefält + sjöfraktens sparade transportväg; inget nytt dispatch-/rörelsesubstrat. |
| Egen budbärare | `/messengers` (world.go1405): own, endpoints/tider; aktuell fysisk passageleg1592–1653, carrier-id vid aboard. Ingen lagrad hexrutt. | Exponera den befintliga modellens A*-väg för aktiv markleg; waiting/sealed ger ingen framåtväg, aboard följer faktisk carriers sparade väg. Detta är inte ny budruttlagring. |
| Egen karavan/sjöfrakt | `/trades`1223: transports.journey/departedTick/dueTick1261, SavedPosition1334 äger current_q/r. DTO1279 saknar path/costs/category/ship-id. | Exponera validerad frusen TradeJourney endast för ägaren. Ursprung/mål i sparad sjöväg kan vara vattenankare en hex från stadskoordinaten. |
| Främmande armé/Host | `/foreign-units` dto28–69: current q/r, owner/typ/stance, heading; FOW191–198. ReadMarch180–184 äger kompass NU. Ingen path/target/trueETA. toward är uppskattning till betraktarens egen stad, inte faktisk främmande ETA. | Rita pil från befintlig heading. Rätta stale kommentar om full rutt i foreign_units.go90. Ingen ny synlighetsregel. |
| Främmande skepp | Samma säkra `/foreign-units`, category naval. | Samma pil, aldrig path; ingen förändring av främmande armé/skepps positionsmodell. |
| Främmande budbärare | Skickas av `/messengers`; aktuell-position-FOW1668–1683, aboard/sealed döljs1662. DTO1449 skickar trots det origin/dest och ISOtider; heading saknas. | Säker foreignprojektion current q/r + ReadMarch-heading under samma grind. Inga egna leg-/carrier-/order-/endpoint-/tidfält på foreign. |
| Främmande karavan/sjöfrakt | Skickas av `/trades`, live-current-FOW1363–1383 för utomstående. DTO läcker endpoints/ISO;1390 tömmer endast cargo+tickETA. Mottagare role=recipient bypassar redan FOW och får känd cargo/ETA (map_trades_mine_test255/385). | Säker foreignprojektion current q/r + kompass NU, inga rutt/endpoint/tidfält. Mottagarundantaget avstäms med Claude före ändring; cargo/roll och ordinarie handelsdetaljer behöver inte försvinna. |

Keryx `sightings`: idag bara `/foreign-units` + provinser (cmd_sightings.go104/122); armé/skepps-heading skrivs redan170. Behöver även bud-/karavanernas säkra kartprojektion om servern lägger till riktning. Ingen ny kommandohandling.

Relationer i befintlig MVP: egen vs främmande neutral/okänd. `FOREIGN_ACCENT` är befintlig violettgrå, vald tidigare för diskriminerbarhet mot sprites/terräng; inga förklarade krig/alliansenum finns på dessa kartpayloads. Egen färg + framåtstreck, främmande färg + riktningspil ger redundant form; slag bärs av olika streckmönster. Två BILD-relationer behöver inte uppfunnen diplomati. Avgränsningen är skickad till Claude.

## Baslinje

[JS på oförändrad746398ed](baseline-js.log). Server-/process-/bildbaslinje tas före ändring av respektive yta. Befintlig isolerad rigg ska styrkas med sin egen healthz/provenance; sourcecommit och gammal process får inte sammanblandas.

## Experiment och grindar

Pågår. Inventeringen ovan skrevs före produktionskod. BILD till Timothy efter prov och ny process.
