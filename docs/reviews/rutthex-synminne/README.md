# Rutthexens synminne

## Sammanfattning

V2-prefix sparade enbart en nådd rutthex; dess fulla synfält kunde försvinna mellan kartläsningar. Varje nådd enhetshex använder nu samma `province.SweepLiveRadius` som levande sikt. Avgångshexen räknas från exakt avgång, utan ny namngivningsrätt. Kod `49f1e342`; slutlig rigg `a8ce5a41`.

Provenance: riktig acceptansprocess `49f1e342+route-sight`, migration217, statusok. `bild/proof.json` och `bild/server.log`. Server/web hade964 identiska produktionsfilhashar genom scenariot. Deploybevis tillkommer separat.

## Slice-kontrakt

Blockerar geografidelen av kedjegrinden. Problem: missat synfält mellan kartläsningar. Spelarsanning: det varje faktiskt nådd enhetshex ser förblir kartminne. Invariant: befintliga siktregler, ingen framtida ögonposition eller ny namngivningsrätt. Scope: prefixkonsument, DB-regression, bevittnad historisk backfill. Non-scope: fart/radier/order, fraktens siktsemantik, speltestets utfall. Acceptans: full sikt vid gles polling; exakta gränser; land/skepp skiljs; idempotens; rättning endast med belagd passage. Stopp: saknad historisk färddata ersätts aldrig med gissad väg.

## Baslinje och experiment

- `baseline-red.log`: färsk217 på oförändrad produktionskod577c63ab, nytt DB-prov assertion-rött: passerad mitthex tappade radie2.
- Två äldre tester krävde att en framtida rutthex var okänd även om den låg inom synhåll från nådd position. Justerade förväntningar: nådda hexar ska vara kända; framtida ögon får inte användas. `TestTimedAdvanceStopsBeforeFutureHexMemory` använder nu en yttre sentinel som bara framtida målögat kan se.
- Testverktyget kunde träffa Postgres initfas mellan temporär socketserver och permanent server. Egen commit7bc48baf väntar på TCP-lyssnaren före DB-setup. De tidiga socketfelen är riggfel, inga regressioner.
- Acceptansriggen antog först fasta kartkoordinater, därefter felaktigt att Map returnerade objekt i stället för lista, därefter glömdes starteskortens extra ögon. Alla tre försök underkända och loggade i `rig-attempts/`. Slutriggen väljer existerande platser och isolerar ett enda öga före ordern. Inga produktionsändringar krävdes.
- Första backfill-mutanten gav ett oanvänt variabelnamn och räknades inte. Slutmutanten kompilerar och är assertion-röd. `mutants/proof.json` anger exakta ersättningar.

## Historisk backfill

`cmd/backfill-route-sight` använder endast kvarvarande v2-rutts exakta hash och dess `place_reach_progress.last_milli`. Den söker ingen ny väg och ger ingen namnrätt. Saknad/missmatchad historik hoppas över. Operatören måste verifiera historiskt ägarskap. Standard är rollback; explicit `-apply` krävs för commit.

Den ursprungliga tick1351–1377-rutten för cec6/26df är ersatt i drift. Timothy belade redan tre ögon: (51,17),(50,17),(50,16), därför finns deras uttryckliga landvittnen i `verified-land-eyes.json`. Ingen rekonstruerad helrutt påstås. Dagens privata driftkopia visar att de fyra rapporterade luckorna redan är kända; samtliga återvinningsbara v2-rutter för båda ägarna ger också noll nya rader. `upgrade-proof-all.json`: torrkörning lämnar DB oförändrad, apply+omkörning idempotenta, alla77 andra tabeller bytehash-identiska. Inget behov av liveskrivning för dessa redan återställda minnen.

## Grindar och täckning

| Grind | Bevis |
|---|---|
| Kod | Färska DB-prov; full Go-svit, vet,803JS; fyra fysiska assertion-röda mutanter, exakt restore och färskt grönt. |
| Visuell | `bild/route-memory-1280.png` och390: grått bestående minne bakom hosten, levande sikt runt slutpositionen, mörker bortom. Native Firefox, bilder ögonlästa vid1:1. |
| Semantisk | API mitthex `remembered`, yttre sentinel `fog`; DB-minne finns före första kartläsningen. Inga synradier, fart eller namnrätter ändras. |
| Användare | Registrera/join → Keryx march → verklig HTTP/worker/ankomst utan kartläsning längs resan → Keryx Map och webben. Explicit startfixtur: slättkarta, ensam host, känt mål. Inga DB-ingrepp efter ordern. |
| Drift | `deploy-proof.json`: ny processhash,217ren, samma värld/konton/städer/enheter/varurader/fysiska enhetsdata och fyra byteexakta assets. |

Nya prov täcker glest pollad landrutt, radie2, skeppets kust/inlandsgräns, avgång och första inträde före/på exakt offset733, stopp vid mellanliggande strid, backfill-cutoff, torrkörning/rollback, idempotens och stale hash. Bestående namngivningsprov körs tillsammans med dem. Kärnfilens mutant: ersätt `if v.displayID == v.id {` med `if false {` → `TestRouteHexRetainsFullSightBetweenPolls`. Backfillfilens mutant: `if int64(at) <= last {` → `if int64(at) <= last || true {` → `TestBackfillOnlyWitnessedPrefixAndDryRunRollback`.

## Avgränsningar

Ingen allmän återvinning av redan överskrivna rutter utan separat passagebevis. Ingen ändring av fraktens ögon eller gamla versionslösa resors tolkning. Nya prov inventerar inte alla bergs-/havssiktlinjer på nytt; samma befintliga siktägare används och dess befintliga prov ingår i fullsviten. Ingen ny handling/API införs: webb och Keryx får rättat minne från samma Map-svar; Codex befintliga beskrivning av bestående sikt gäller fortfarande.
