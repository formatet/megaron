# Macchia-timmer — slice 22, mätning före beslut

## SLICE-KONTRAKT
Problem: timmer finns på få terränger och kan saknas inom en stads upptagningsområde.
Spelarsanning: rapportera faktisk åtkomst före eventuell balansändring.
Invariant: live läses i READ ONLY-transaktion; ingen migration, produktion eller spelorder ändras.
Scope: separat mätkommando, SQL-uttag, analys och rapport i denna katalog.
Non-scope: implementering av föreslagna rater eller nya platser, recompute och deploy.
Acceptans: verkliga live-rater; live-städer och möjliga platser; minst tre färska världar; produktiv radie 2 med centrum exkluderat; reproducerbara data och siffror.
Stopvillkor: **STOPP med siffrorna före migrationen** enligt Claudes START 2026-10-19 08:23.
Bevisplan: oförändrad mapgen, live healthz + repeatable-read READ ONLY JSON, helt fristående analys, två identiska färska genereringar och kalibrerad geometrikontroll.
Kedjegrinden: **bevisar** var geografi → brist möter en timmerspärr; inga balansbeslut fattas.
Bas: c761626b; live healthz 2026-10-19 svarade commit c761626, migration188, statusok.

## SAMMANFATTNING — STOPP före migration

Oförändrad kodbas c761626b. Liveuttaget är en konsistent `REPEATABLE READ READ ONLY`-transaktion, migration188, tick601; healthz svarade commitc761626/migration188/statusok. Inga skrivningar, migrationer eller recompute körda. JSON innehåller endast världens geometri/regler och stadsnamn/population, inga kontotoken.

**1 av 10 aktiva live-städer saknar timmerhexar: Amnissos.** Föreslagna terränger skulle ge den 11 möjliga timmerhexar. Tre färska64×64-världar ger tillsammans **690 av2635 terrängmässigt grundningsbara platser utan timmer (26,19%)**; med föreslagna terränger återstår78/2635 (2,96%). Detta är tillgång, inte full produktion eller självförsörjning.

Viktig premissrättelse: live ger **bara olivlund timber1/tick per gubbe**. Cederskog ger **cedar1**, ingen timberrad. Ceder har inte räknats som timmer. Sten finns redan på alla tio landterränger:0,5 på åtta,1 på kullar,2 på kalkberg. Det finns ingen saknad landterräng att tillföra0,25sten på.

## EXPERIMENT — produktiv radie2

Centrum exkluderas enligt `hexgrid.Ring`, `economy/placement_yield.go`; det finns18 produktiva hexar på en komplett radie2, inte19. Vatten och kartkant kan minska antalet landhexar. Alla tabeller räknar hexar med positiv ogatad timmertakt, oberoende av antal arbetare eller byggnader.

| Karta | Terrängmässigt möjliga platser | Utan timmer nu | Median timmerhexar | Min–max | Utan timmer om maquis/kulle/floddal tillkommer |
|---|---:|---:|---:|---:|---:|
| live-f7c90afa-4653-46a6-b14c-473a268efa3e | 894 | 249 (27.85%) | 3 | 0–14 | 28 |
| fresh-1009 | 899 | 232 (25.81%) | 2 | 0–17 | 29 |
| fresh-2018 | 859 | 238 (27.71%) | 2 | 0–17 | 26 |
| fresh-3027 | 877 | 220 (25.09%) | 3 | 0–17 | 23 |

Ett separat rumsligt urval packar platser med minst5hex mellan centra (värdspawnens spacing), deterministiskt hashordnat. Det är ett urval **potentiella** städer, inte en körning av spawnens last-/malm-/slumpordning eller en rekommendation om spelarantal.

| Karta | Rumsligt skilda platser | Utan timmer nu | Median | Utan timmer med förslaget |
|---|---:|---:|---:|---:|
| live-f7c90afa-4653-46a6-b14c-473a268efa3e | 52 | 18 | 2 | 4 |
| fresh-1009 | 56 | 15 | 2 | 5 |
| fresh-2018 | 58 | 22 | 2 | 6 |
| fresh-3027 | 57 | 20 | 2 | 4 |

### Alla aktiva live-städer

| Stad | Timmerhexar nu | Med förslaget | Separata cederhexar |
|---|---:|---:|---:|
| Phaistos | 5 | 6 | 0 |
| Thalassa | 4 | 4 | 0 |
| Amnissos | 0 | 11 | 0 |
| Kydonia | 5 | 13 | 0 |
| Mallia | 1 | 1 | 3 |
| Tylissos | 2 | 11 | 2 |
| Scheria | 5 | 5 | 0 |
| MalliaWest | 1 | 1 | 3 |
| Amnisos | 8 | 8 | 0 |
| Itanos | 1 | 4 | 0 |

### Platser och framtida byggnadsgrind

`hex_rules.go`: olivlund har1 timmerplats och lumbermill som relevant byggnad. Maquis, kullar och floddal saknar uttrycklig timmerrad; om endast en production_rules-rad lades till skulle generiska `HexFallbackCap=2` ge två basplatser men **ingen lumbermill-koppling**. En framtida implementation måste därför uttryckligen bestämma basplatser och eventuell sågverksbonus/gate; en migration ensam motsvarar inte automatiskt olivlundens regel. Inget av detta ändras här.

Scenariot i tabellerna räknar endast om terrängen över huvud taget skulle erbjuda timmer vid positiv takt. Talet0,33 påverkar inte antalet åtkomliga hexar. Det är inte bevis för att en arbetare kan placeras via HTTP ännu; ett sådant test hör till implementationsslicen efter beslut.

## GRINDAR OCH REPRODUKTION

- `go build` och `go vet ./cmd/material-measure` gröna. Verktyget använder verklig `world.GenerateMap`, inga produktionsmodifieringar eller databas.
- Alla tre färska kartor genererade två gånger: byte-identiska JSON. Begärda respektive effektiva seeds finns i varje fil.
- Oberoende SQL över live-DB ger identiska timmerantal för samtliga10städer. Båda SQL-recepten har explicit READ ONLY.
- Kalibrering:18hex, centrum bort, radie2 in/radie3 ut, ceder skiljs från timmer. Fysisk mutation radie2→1 ger AssertionError; byte-exakt återställning och ny analys grön. Se checks.json.
- Inga spelarflöden, migrationer, UI eller driftsändringar ingår. BILD ej tillämplig.

```sh
# Från server/ i detta arbetsträd:
go build -o /tmp/megaron-material-measure22 ./cmd/material-measure
# Från reporoten, för seed1009,2018,3027:
/tmp/megaron-material-measure22 -seed 1009 > docs/reviews/macchia-timmer/fresh-1009.json
python3 docs/reviews/macchia-timmer/analyse.py
# Liveuttaget kan upprepas separat; fångar då ett nytt tick/population:
ssh root@10.0.1.92 "runuser -u postgres -- psql -X -d poleia -Atq -v ON_ERROR_STOP=1" < docs/reviews/macchia-timmer/live.sql
```

## METODISK LÄRDOM

Separera vara från besläktat material (ceder≠timmer), produktivt upptagningsområde från geometrisk disk och geografisk potential från faktiska spawn-/produktionsutfall.

## KÄNDA AVGRÄNSNINGAR

Alla terrängmässigt möjliga platser innehåller överlappande upptagningsområden. De är inte lediga eller alla möjliga att grunda samtidigt. Befintliga provinser/värdar, landmassans timberfilter, mat, avstånd till grannar, ägda/delade hexar och faktisk bemanning filtreras inte i kandidattabellen. Stadsraderna visar potentiell terrängåtkomst, inte exklusiv nyttjanderätt eller befintlig produktion. Tre seeds räcker för ett transparent urval, ingen populationsstatistik över alla frön. Den geografiska kontrollen visar inte vägar eller färd över vatten. Färsk mapgen använder live-reglerna som oförändrad ratepremiss; ingen färsk PostgreSQL-värld behövs för denna geometriinventering.

**Nästa handling:** Claude/Timothy bedömer siffrorna och uttrycklig timmerplats/lumbermill-regel; 21b-bilder prioriteras före eventuell implementation22. Hash+STOPP, inga migrationsfiler.
