# ORDER 6 — hela förband vid disband

## Sammanfattning
Ett val av soldatantal kunde krympa en godtyckligt vald kohort. Disband väljer nu ett eller flera hela förbands-id:n och återför deras verkliga kvarvarande personer. Fyra ytor uppdaterade på `codex/hela-forband`, bas `7c2a906b`. Full fresh Go/mig198 GREEN (world154.194s),704 JS/vet GREEN, named mutation RED→restored GREEN. Ingen merge/push/deploy; BILD väntar Timothy via Claude.

## Kontrakt före kod
Kontraktsskissen skickades till Claude 2026-10-22 02:16 före implementation.

| Fält | Kontrakt |
|---|---|
| Problem | Disband tog ett antal män per typ och krympte förband minsta-först. |
| Spelarsanning | Välj en namngiven hel gubbe/kohort eller ett helt skepp; se hur många personer som återvänder till staden. |
| Invariant | Ingen partiell krympning, ingen dubbel återföring, inget främmande eller bortrest förband avvecklas; silver orört. |
| Scope | Disband API + lås/population, Keryx, city/war, Codex, verblista, relevanta prov och Firefox BILD. |
| Utanför | Ny silverpolicy, nya event/notiser, andra enhetstyper, ny rekrytering/hemvist, livevärld. |
| Acceptans | Helt100, skadat41, främmande403, marsch422, skeppscrew, laid_up0, gammal mängd400; namngiven mutation. |
| Grind | Funktion i kod; BILD i Firefox före merge; TEXT i speltest. |
| Kedja | Bevisar flytt av befintligt folk till stadens produktion i geografi→brist→brons→elit. |

## Baslinje och implementation
Baslinje på ny PG16/mig198: ursprungliga Disband API/Keryx-prov gröna (`baseline.log`). Gamla `disbandPlan` och dess partiella konsumtionsprov är borttagna tillsammans med mängdvägen.

- Samma route: `POST /worlds/{w}/provinces/{p}/disband`, body `{"unit_ids":["uuid",…]}`. Tomt, dubbletter, ogiltigt id och gamla mängdnycklar nekas med 400. Gamla former får `disband a whole unit by its id`.
- Lås aktiv egen stad → förband i stabil id-ordning. Kontrollera värld/ägare/stad/status och samma sex typer som tidigare. Validera hela batchen före skrivning. Återför faktisk land-size eller naval-crew; laid_up återför 0. RecomputeProduction och commit i samma transaktion.
- Svar: `disbanded:[{id,type,display_name,people_returned}],pop_restored,population,settlement_name`. Ingen befintlig eventtyp ändras; det gamla kommandot skrev inget event och detta behålls.
- Keryx `disband --unit ID/unik namn` (upprepbar), valfri `--province`. Namn/id-prefix matchas bland egna förband, tvetydighet vägras. Kvittensen säger `returns N people to <stad>`. Typ/mängdflaggorna är borttagna.
- City → Garrison använder egna förbandsrader med serverns namn, en gubbe och knapp per förband, inga mängdfält. Förbrukad/skadad kohort återför bara verkliga personer. Den gamla aggregattabellens `count×POP_COSTS` togs bort från denna yta eftersom den gav fel antal personer (100 spearmen visades som 500).
- Webben skickar ett id; pending-skydd hindrar dubbelklick, sena svar får inte skriva i en annan stad/värld. Garnisonsfliken återöppnas efter omladdning så kvittensen syns. Befintlig Umami-route `army_disbanded` används.
- Codex `units.md` och vaultens `temenos_enheter`/`megaron_verblista` uppdaterade. Silverpolicy införs inte; 77 silver står kvar i DB-provet.

## Experiment och funktion
`TestDisbandSelectedWholeUnitAndDamagedSurvivors` väljer full100 och lämnar annan41 orörd, avvecklar därefter41, nekar upprepning och kontrollerar oförändrat silver. `TestDisbandAtomicOwnershipAndPresenceChecks` täcker främmande403, marching/training, saknad stadsanknytning och Host422 med hela batchen orörd. `TestDisbandRejectsLegacyQuantityAndDuplicateWithoutChange` täcker gamla mängdformer och dubbletter400. Befintliga crew/roundtrip/laid_up-prov är flyttade till id-formen.

Mutation: ersätt hel avveckling med `size=GREATEST(size-20,0),status=garrison`. Namngivet test blir RED, då100-kohorten står kvar som80. Ursprungliga bytes återställda, samma prov + övriga Disband API/Keryx GREEN på ny PG16. Se `mutation-red.log` och `restored-green.log`.

## Firefox BILD och scenario
Recept: `python3 docs/reviews/hela-forband/browser_check.py`. Riktiga city.js/war.js, riktiga drawer-klasser/CSS, isolerad lokal HTTP-fixtur. Inga livekonton, inga ändringar i delad acceptansstack eller spelvärld. DB-/API-kontraktet prövas separat mot riktig PostgreSQL, inte genom en fejkad DB i browsern.

För varje bredd1000/390 återställs fixturen och hela scenariot körs två gånger: öppna stad→Garrison, läs100/41/20 personer, klicka just skadad41, verifiera exakt ett `unit_ids`-anrop,41 återförda,1041 population, två kvarvarande förband och synlig kvittens. Båda omkörningarna rena, noll page errors (`browser-proof.json`).

BILD1:1: `firefox-1000-before.png`, `firefox-1000-after.png`, `firefox-390-before.png`, `firefox-390-after.png`. Namn är läsbara, inga UUID exponeras. Granskade i1:1 av Codex; Timothy avgör BILD före merge.

## Kända avgränsningar
Disband har samma tillåtna typer/statusar som förut. Denna slice bygger ingen ny hantering av last/passagerare eller nya händelser. Gamla klienter som skickar mängder måste använda förbands-id i stället och får tydligt400 tills de uppdaterats.


## Grindar / slutligt bevis
| Grind | Utfall |
|---|---|
| Kod | `GOTEST_PORT=57900 tools/gotest.sh`: ny tom PG16/mig198, hela sviten GREEN, world154.194s; `full-fresh-go.log`. |
| Webb / verktyg |704/704 JS GREEN; `js-summary.log`. `go -C server vet ./...` GREEN; `vet.log`. |
| Funktion | Hel100, skadad41, annan kohort orörd, främmande403, ej närvarande422, gamla former400, navalcrew60, laid_up0, silver77 oförändrat. Fysisk partiell-krympningsmutation named RED→restored GREEN på separata nya databaser. |
| BILD | Firefox1000/390, två rena körningar perbredd, riktiga stadspanelen och riktiga knappar. Bilder skickade till Claude; Timothy avgör före merge. |
| TEXT | Personåterföring och förbandsval prövas i speltest efter integration. |
| Drift | Ingen deploy eller ändring i den levande världen. |

Proveniens: egen worktree `/home/tk/wt/codex-hela-forband`, bas `7c2a906b`, färska testdatabaser via tools/gotest.sh. SHA256 för testade ändrade server-/webbfiler i `tested-files.json`; Go-koden ändrades inte under/efter helsviten. Efter helsvitstart justerades endast webbens0-crew-fallback för ListUnits `omitempty`; slutliga704JS och båda rena Firefox-omkörningarna kördes därefter. Inga främmande arbetskopior eller tjänster ändrades. Delad acceptansstack lämnades orörd; browsern använder isolerad HTTP-fixtur, DB/API-testerna riktig PostgreSQL.
