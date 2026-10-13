# Tidiga tempelgåvor — inventering och receptförslag

## Sammanfattning

Status: analys klar; inget produktionsrecept ändrat. Gren `codex/tempelgava-tidiga-varor`, bas `91a63709`.
Read-only dev-provenance från processen: `commit=91a6370`, migration `179`, status `ok`.
Värld `f7c90afa-4653-46a6-b14c-473a268efa3e`, forming, tick 150, 64×64.

Templet använder idag 2 olja + 1 vin som referens, men tick och prognos har redan
en ersättningsväg med andra lokala varor av samma gudomliga värde. Status/webb/keryx
visar bara olja/vin och kan därför påstå att ett tempel inte är matat när tick kan mata det.
Orderns påstående att avsaknad av olja/vin alltid ger noll är inte sant för dagens kod.
Den höga referenskostnaden och den felvisande statusen är däremot konkreta problem.

## Slice-kontrakt

| Fält | Innehåll |
|---|---|
| Problem | Tidiga tempel erbjuds med ett olja/vin-recept och statusen ignorerar befintliga ersättningar. |
| Spelarsanning | En stad med lokal tidig produktion ska kunna mata sitt tempel och se det faktiska villkoret. |
| Invariant | Gåvan tas bara från templets egen stad; befintlig eventsemantik består. |
| Scope | Denna fas: read-only geografi, produktionsrater, läsarinventering, talförslag. |
| Non-scope | Ingen balansändring, spawnändring, migration, merge eller deploy. |
| Acceptans | Mät alla möjliga startplatser; redovisa saknade varor; jämför mängder med verkliga rater; inventera fyra ytor; reproducera mätningen. |
| Stopvillkor | Ordern kräver hash+STOPP efter förslag. Exakt universell lokal försörjning kan inte uppfyllas med de fyra varorna på dagens karta. Timothy avgör. |
| Bevisplan | Faktisk dev-snapshot + oberoende SQL + offline replay. Kod-/användarprov hör till senare implementation. |

Kedjegrind: blockerar religionens tidiga nåbarhet för en ny stad.

## Baslinje och geografi

Populationen är **831 möjliga join-startplatser**, efter terrängfilter, timmerbärande
landmassa och befintlig host-/provins-clearance. Last-/metallrangordning påverkar
sannolikheten att väljas, inte populationen här. Rise-exkludering av tidigare landmassa
ingår inte. Catchment är de 18 arbetade hexarna inom radie 2, centrum undantaget.
Ingen FOW-, ägar-, belägrings- eller senare koloniplatsgaranti påstås.

| Lokal förutsättning | Platser | Andel |
|---|---:|---:|
| Timmerbärande skog | 620 | 74,61 % |
| Hav i catchment | 588 | 70,76 % |
| Fiskproduktion, även floder/vadställen | 602 | 72,44 % |
| Silverfyndighet, gruva krävs | 50 | 6,02 % |
| Kornproduktion | 798 | 96,03 % |
| Korn eller fisk | 818 | 98,44 % |
| Korn och timmer | 607 | 73,04 % |
| Någon av korn/fisk/timmer/silver | 829 | 99,76 % |

Två startplatser, `(4,65)` och `(10,72)`, har ingen av de fyra varorna i arbetat
catchment. Axiala r-koordinater kan överstiga kartans nominella höjd.
De saknar även lokal korn/fiskproduktion. Host-spawn väljer avsiktligt inte bara
självförsörjande grundningsplatser (`join.go`); spelaren ska leta efter sin plats.
Ett fast recept som kräver alla fyra varorna kan alltså inte uppfylla ordern.

Aktuell produktion per gubbe och tick, utan förbättringsbyggnader:
korn 1/3 på kullar, 1 på slätt, 5/3 i dal, 8/3 i delta;
fisk 1 vid kusten och 1/2 i flod/vadställe; timmer 1 i olivskog.
Silver ger 1 på kullar eller 2,5 på kalkberg med nivå-1-gruva enligt byggnadsregeln;
denna världs nåbara silver ligger på kullar. Dagens `labor_value` är 1 för alla fyra
tidiga varor samt olja/vin. Fertilitet multiplicerar inte placeringens hex-yield.

## Förslag till Timothy

**Rekommendation: referensgåva 0,25 korn per tempel och tick, med lokala ersättningar
i fisk/timmer/silver till samma gudomliga värde; inget obligatoriskt silver.**
Vid normala gudomliga värden motsvarar det 0,25 av valfri av dessa varor,
eller en blandning med samma totalvärde. Scarcity följer ett gemensamt referensrecept
i tick/prognos/status; faktiska mängder kan skilja sig när gudomliga värden skiljer sig.
Allt-eller-inget behålls. Hur alternativ väljs måste vara deterministiskt och synligt
innan en stad debiteras; en automatiskt vald silvergåva får inte döljas i en kornrad.

Målkurva: stående gåva högst 0,25 oförbättrade slätt-/timmer-/kustgubbars produktion,
så att tidig religiös progression inte kräver raffinering eller lokal silvergruva.
På den svagaste kornterrängen blir det 0,75 gubbars produktion; faktiska gubbar är
heltal. Detta är ett nytt talförslag, inte ett tidigare godkänt balansbeslut.
Gamla referensens 3 dagsverken blir 0,25, en minskning med faktor 12.
Staffad kult krävs fortfarande för positiv vinst; netto beror även på förfall.

Alternativ att ta ställning till:

1. **Acceptera platsletandet (rekommenderat).** Receptet fungerar på samtliga 818
   startplatser med lokal matproduktion och ytterligare 11 med någon annan tidig vara.
   De två karga platserna kräver att Host flyttar före grundning eller senare försörjning.
   Spawn/karta/produktion lämnas som idag.
2. **Kräv lokal försörjning på exakt alla spawnplatser.** Då behövs ett separat
   kanonbeslut om spawnfilter, ny produktionskälla eller gratis stående gåva där varor
   saknas. Det ändrar grundningsfasens regel och ryms inte i ett receptbyte.

Mängdval: 0,5 eller 1 dagsverk är möjliga men tar 1,5 respektive 3 gubbars
kullproduktion per tick. Jag rekommenderar 0,25 enligt riktlinjen att välja generöst
när de tidiga ekonomiska trösklarna annars blockerar spelaren.

## Läsare och genomförande efter beslut

| Yta | Aktuell kod / nödvändig förändring |
|---|---|
| Temenos tick | `internal/kharis/tick.go`: OfferOilPerTemple/OfferWinePerTemple, applyTempleOffering, feedTempleBySubstitution, processMaintenance. Delad recept- och gåvoplanering; lokal debitering i transaktion utan delvis betalning. |
| Prognos/scarcity | `internal/kharis/project.go`: scarcityFromValues, projectFedTemples, canSubstituteFeed. Samma recept/plan som tick; tick.go templeOfferScarcity läser redan scarcityFromValues. |
| Status/API | `api/handlers/province.go`: templeOfferRow/oil_needed/wine_needed/fed och temple_offers. Fed ska följa faktisk gåvoplan, inte bara traditionell olja/vin. |
| Keryx | `cmd/keryx/cmd_status.go`: templeOilPerTick/templeWinePerTick, sinkCapacities, parser av temple_offers, kultutskrift och råd. `cmd_status_surplus_test.go` innehåller gammalt underhållsantagande. |
| Webb | `ui/drawers/kult.js`: Temple offerings, stat-row/name+needs samt kultprognos i kult_kharis.js. Visa faktiskt gemensamt recept; rätta saknat mellanrum. |
| Codex | `web/static/codex/temples.md`, `kharis.md`: olja/vin-påståenden. |
| Event | KharisOffering visar fed/total/fraction/devotion/scarcity/valuation/gain; innehåller idag inga olja/vin-mängder att tolka om. Eventhistoriken behålls. |
| Vault | `temenos_kharis`, `temenos_balans_spakar` §9 och religionens beslutshem uppdateras när nytt recept beslutas/byggs. |

Sökningen hittade också historiska migrationskommentarer (`092`, `103`, `115`, `136`),
ekonomitestets kommentar `wine_beyond_hills_test.go`, och många olja/vin-rader om
skördebonus och tillväxt. Migrationerna förblir historiska; skörd/tillväxt ändras inte
av tempelreceptet. `kharis/labor_flow_test.go` mäter dagens olja/vin-scarcity och måste
byta meningsfull fixtur i implementationen.

## Grindar och reproduktion

| Grind | Utfall |
|---|---|
| Analyskod | Offline replay och separat SQL ger samma 831 kandidater och sju täckningsmått. |
| Visuell | Ej ändrad; Firefox 1280/390 före senare implementationsleverans. |
| Semantisk | Statusens olja/vin-gate och tickens ersättningsväg skiljer sig; dokumenterat. |
| Användare | Ingen produktionsändring gjord; naturligt webb/keryx-scenario krävs i nästa fas. |
| Drift | Endast BEGIN READ ONLY, ingen deploy, ändring av startvillkor eller forcerad tick. |

Kör från denna worktree:

```sh
python3 docs/reviews/tempelgava-tidiga-varor/measure.py
python3 docs/reviews/tempelgava-tidiga-varor/measure.py --snapshot docs/reviews/tempelgava-tidiga-varor/snapshot.json
ssh root@10.0.1.92 'runuser -u postgres -- psql -d poleia -X -qAt -v ON_ERROR_STOP=1' < docs/reviews/tempelgava-tidiga-varor/validate.sql
```

`snapshot.json` är rådata; `measurement.json` innehåller varje kandidats koordinater
och bästa lokala per-gubbe-rater; `independent-sql.json` är oberoende räkningsbevis.
Ingen full Go/JS-svit eller produktionsmutation påstås för denna rena analysfas.
