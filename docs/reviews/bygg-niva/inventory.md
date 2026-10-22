> Historisk inventering före kod. Claude godkände quote-/hex-formatet 18:42; implementation pågår enligt det beslutet.

# ORDER 17 — inventering före kod

Bas `ea97b332`, egen gren `codex/bygg-niva`, worktree `/home/tk/wt/codex-bygg-niva`.
ORDER13 är överlämnad separat som `b2729e15`; inte mergad när denna inventering görs.
**Endast läsning, dokument och lokal körning av oförändrad formatterare. Ingen produktionskod, ingen liveåtgärd.**

| Kontrakt | Konkret innehåll |
|---|---|
| Problem | Befintligt varv L1 avslås på cedar för L2, men spelaren tror att L1 kräver cedar. |
| Spelarsanning | Varje byggförsök/listpris anger befintlig nivå, målnivå och just den byggplatsens nästa kostnad. |
| Invariant | Priser/grindar, betalare, avdrag, queue/refund och LevelledSpec ändras inte. Structured missing/insufficient_goods behålls för andra konsumenter. |
| Scope | Handlerns build-avslag/quote-data, Keryx build/client, City Construct och dess feltext, Codex buildings; relevanta tester. |
| Non-scope | Balans, frakt/handel, rekrytering, serverägd kostnadsformel och ORDER13. |
| Acceptans | L1→L2-varv säger båda nivåer och cedar4; max anger aktuell nivå; list/json anger nästa pris i vald stad; två hexbyggnader behandlas var för sig; webbpris matchar avdrag. |
| Stopvillkor | Behöver beslut om konkret quote-format/per-hex-representation eftersom dagens stadsaggregat är otillräckligt, se nedan. |
| Bevisplan | Named RED mot gammal text, verkliga HTTP-prov, CLI och webbformatprov, fysisk mutant, full fresh Go57900 en slutlig gång, JS/vet, BILD för ändrad webb, TEXT i playtest. |

## Uppmätt kodläge

1. `province.go:1304–1330` väljer redan korrekt `LevelledSpec(current+1)`. För hex-bound läses **den valda hexens** nivå; stadsbyggnader använder typens nivå. Max-avslag har ett tal men skiljer inte uttryckligen aktuell/mål eller stad/hex. Vägg har separat ladder.
2. `province.go:1391` skickar generiskt `writeGoodsError`. `helpers.go:34` returnerar `{error:"insufficient_goods",missing:[...]}` utan byggnad/stad/nivå. Silver har separat felväg omkring1402 och behöver samma byggkontext.
3. Keryx `client.go:222` och webben `ui/format.js:103` bygger **egna** meningar ur `missing`. En servermening ensam löser alltså inte hela kedjan; båda måste konsumera byggkontexten samtidigt som generiska fel fungerar vidare.
4. `cmd_build.go:76–180`: `--list` hämtar bara global `/buildings`. Stadargumentet används inte för att läsa stadens byggnader. `--json` returnerar också bara rå global katalog. Kolumnen är `Costs (L1)`; upgrade_costs finns i katalogen men visas bara som cederstege vid sidan av.
5. **Webben är också fel:** `province.go:266–305` använder nästa kostnad i `can_afford`, men **själva pristexten** i `city.js:467` använder alltid `entry.costs` (L1). Anropet:656 lämnar bara walls/siloLevel. Lokal körning av oförändrad `buildingOptionsHTML` med shipyardL1 och katalogens L2-pris gav `Shipyard — 24 timber 12 stone`, se `baseline-web.txt`; rätt nästa pris är 48timber+24stone+4cedar. Detta är formatterarbevis med uttrycklig fixture, inte live-API-mätning.
6. **Hexnivåer kräver plats:** fyra hexbundna typer kan finnas flera gånger i samma stad. `builtLevels` i province-GET använder MAX per typ; det får inte återanvändas som en entydig aktuell nivå eller nästa pris. Farm på hex A L1 och farm på B L2 måste visa olika uppgraderingar, och ett nytt fält C ska fortfarande erbjuda L1. `hexBuildOptionsHTML` visar redan destinationens L+1 men inte pris och erbjuder även L4 om rå byggnad är L3.
7. Codex `buildings.md` säger redan att samma hexbyggnad upprepas för uppgradering och att hamn/varv/tempel vidare behöver cedar; allmän text behöver göras uttrycklig: samma byggnad igen höjer en nivå, **nivå2+** för dessa tre kräver cedar. Inga kostnadstal ändras.

## Konkret förslag till Claude före kod

**Behåll en prisägare:** en quote i serverns befintliga province-readmodel/placement-options använder `LevelledSpec`, väggens `WallLevelSpecs` och den aktuella byggplatsen. Quote innehåller type, current_level, next_level (tom vid max), at_max_level, costs, cost_silver, duration_game_days och hex-koordinater för hexbyggnader. Ingen klient skriver en ny materialmultiplikator eller cederregel.

- Stadsbyggnader: en quote per typ i province-GET. Befintliga `can_afford` kan utökas/ersättas kompatibelt med quote-fälten så kostnad och betalbarhet delar samma faktiska underlag.
- Hexbyggnader: en quote per vald byggtyp och giltig ny/befintlig hex i placement-options. Keryx visar befintliga hexars nivå→nästa och en tydligt märkt L1-rad för nytt bygge. Webbens hexpicker anger nästa nivå och pris; typväljaren säger att priset beror på vald hex. Ingen MAX-gissning.
- Fel: byggspecifik wrapper kring structured missing lägger till byggkontext utan att byta error-kod/missing-form. Keryx/webb återger t.ex. `Raising Shipyard to level 2 needs cedar (need 4, have 0). Petras already has Shipyard level 1.` Samma kontext för silver/max och vald hex vid behov. Övriga verb behåller generisk formattering.
- Keryx list löser vald stad före läsning och visar current→next/kostnad/tid; json får motsvarande verkliga stadsdata utan att låtsas att global katalog redan är stadens läge. Föreslår bevara befintlig arrayform och katalogfält, lägga till explicita quote-fält.
- Ingen ny endpoint eller skrivning behövs. Scope rör server/CLI/webb/Codex; webbfyndet gör BILD obligatorisk.

Orderfilen tillåter direkt kod om inget oväntat. Jag stannar vid inventeringen eftersom korrekt per-hex-nivå/pris och de två självständiga felrenderarna behöver ovanstående gemensamma kontrakt; en snabb maxnivåjoin eller serverprosa ensam skulle lämna felet kvar. **Claude: godkänn/justera quote-/hex-formatet innan kod.**
