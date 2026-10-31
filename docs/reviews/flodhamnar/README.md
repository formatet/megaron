# Flodhamnar — inventering (Claude/Sonnet 2026-10-31) — STOPPAD före kod

Order `.agents/order-flodhamnar-20261031.md`. Ingen kod ändrad; ordern krockar med koden och med beslut 10-30.

## Inventering: vilken regel avgör kust/hamn/galär i dag

Ordern antar att hamn/galär/grundargåva följer en *snävare* regel än kartans `coastal`. Det stämmer inte:
**alla** ställen läser samma flagga `map_tiles.coastal` / `provinces.coastal` = "landhex som gränsar till valfritt vatten"
(mapgen `hasWaterNeighbour`: coastal_sea, lake, river, river_ford; Timothy 2026-07-29). Ingen "gränsar till Thalassa"-regel finns.

| Ställe | Regel i dag |
|---|---|
| Grundargåva galär: `found_metropolis.go:266`, `create_metropolis.go:205`, `host_position.go:73-76` | `coastal` (alla vatten) |
| Prognos `foundingGifts`/`harbourNote`/`coastalCentre`: `world.go:699,845,865` (läser `view[0].Coastal`) | `coastal` — samma som gåvan, ingen avvikelse |
| Keryx `coastalGroundLine`: `cmd_unit.go:1205` | `Coastal`-flaggan (tyst när centrumhexen är dimmig/okänd) |
| Hamn + varv, byggbeslut: `province.go:1195-1255` | granne som `terrain_is_navigable` (hav/sjö/flod/vad). Varv kräver dessutom väg till Thalassa (rekursiv CTE, `ShipyardLakeRefusal`) |
| Byggkatalog/`requires_coastal`: `province.go:2919`, `combat/build.go:131`, `economy/placement_yield.go:229` | `coastal` |
| Embark/disembark, amfibisk landstigning, handel/budväg: `unit.go:1236,1425`, `unit_arrival.go:314,2318`, `trade_preview.go`, `messenger*.go` | `coastal OR hamn` |

Slutsats: en flodstad får redan hamn och galär i dag, och en stad vid en återvändsflod eller en sluten sjö får det också.
Lykons fall (1,78) var inte en regelsplittring: grannhexar var `coastal`, men centrumhexen gränsade inte själv till vatten, så
`settle` sa inland och `coastalGroundLine` pekade ut närmaste kusthex — avsett beteende.

## Live (read-only, dev-servern 10.0.1.92, 2026-10-31)

5 aktiva städer, 2 `coastal`. Av dem är **0** coastal utan vattenväg till Thalassa. Ingen aktiv stad byter status oavsett
vilken regel som väljs; ingen vinner något (sjöstad är en delmängd av coastal).

## Krock — stoppat, frågor till Timothy

Ordern kan inte göras som en ren *utvidgning* (det finns inget att utvidga). Dess kontrakt är i praktiken en **skärpning**:
- (A) Ordern: "sjö utan väg ut / flod som inte når havet ger inte hamn/galär". Beslut 10-30 (`megaron_styrande_beslut.md:814`,
  `temenos_terrain.md:157`): "helt stängd sjö: hamn ja, varv nej". Direkt krock på hamn vid sluten sjö.
- (B) Galär som grundargåva vid sluten sjö/återvändsflod är i dag ja; ordern säger nej. Ska gåvan skärpas, eller räcker det att
  varvet (kräver redan väg) är gated? En galär i en sluten sjö kan aldrig lämna den.
- (C) Hamn vid flod som når havet: redan tillåten. Inget att bygga.
- (D) Kan en flodgalär segla ut på öppet hav direkt? Utanför ordern; ingen ändring gjord.

Om (A)-(B) ska skärpas: ett predikat `province.HasThalassaWaterway` (Go) + SQL-spegel, återanvänd CTE:n från `province.go:1240`;
konsumenter `foundingGifts`/`coastalCentre`, galärgåvan i `found_metropolis.go`, `coastalGroundLine`; hamngrinden enligt beslut. Inget av detta är byggt.
