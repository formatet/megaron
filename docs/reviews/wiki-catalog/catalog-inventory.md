# Code catalogue inventory — read-only checkpoint

Source: f9926f57; fresh disposable PostgreSQL migration 218 (clean). No production code or API changes. Machine IDs and display names: `entities-source-inventory.json`; full migrated goods rows: `goods-fresh-schema.json`.

| Kind | Current coverage target | Authoritative owner / exposure |
|---|---:|---|
| Forces | 7 | `internal/unit/model.go` type constants + `DisplayName`; `province.UnitSpecs` supplies six recruitable types. `GET /api/v1/units` supplies recruitment costs, batch sizes, game days and requirements, but excludes Nomadic Host. |
| Buildings | 16 | `province.BuildingSpecs`, `BuildingDisplayName`, `LevelledSpec` and `WallLevelSpecs`; `GET /api/v1/buildings` supplies level costs, duration, purpose and prerequisites. |
| Terrain | 14 | `world/model.go` Terrain constants; labels currently `render/map.js:TERRAIN_LABELS`. Movement owners `province.TerrainMoveTicksFor`; economics production rules. No global terrain catalogue API. |
| Gods | 4 | `religion.TempleGods`, `TempleGodByID`, `TempleName`; no global public catalogue API. |
| Rites | 5 current, 23 retained definitions | `religion.PrayerSpecs` has 18 original plus five current definitions; `CulturePrayers[province.MVPCulture]` selects the five. Owner-private `/cult` and province projections expose actual city availability; these are not global catalogues. |
| Goods/roles | 18 DB rows | 14 active, four parked. Twelve public offer goods from `handlers.tradeableGoodsCatalog`; see distinctions below. |
| Concepts | No exhaustive code catalogue | Capability registry has 34 checker functions, of which eight kingdom verbs are gated by `KINGDOMS_ENABLED`; not an exhaustive concept registry. |

Goods need semantic classification rather than blind status filtering. Twelve current offer goods are bronze, cedar, copper, fish, grain, incense, livestock, oil, stone, timber, tin, wine. Silver is active and transferable currency but intentionally excluded from silver-for-goods offers. `cult` is active in the table but **not a commodity**: migration 094 retains only its labour FK anchor; devotion produces no stock. `shipbuilding` is parked but represents **implemented** Shipyard work, not a future feature (migration 204, zero weight, no inventory/production rule). The remaining parked rows are horses, pottery and purple; parked status proves their exclusion from the active offer catalogue, not permission to invent future mechanics. `economy.IsShippableGood` uses weight, not the offer predicate; keep these separate.

Retained distinctions: `trireme` is a Galley input/display alias, not an eighth force. Retired unit identifiers and labels are explicitly listed in `unit.RetiredIdentifiers` / `RetiredDisplayLabels`. `tower` and `bronze_wall` occur in the display-name map but not BuildingSpecs; Bronze Wall is a wall tier, not a seventeenth buildable type. Other cultures remain in code but `province.NormaliseCulture` selects minoan; their rites and the original three Minoan prayers are not current MVP article requirements. `lake` is not in the terrain enum; do not infer implemented terrain from prose.

## Proposed guard and generated facts (not implemented)

Add article index entity references `{kind,id}`. A Go coverage test reads index JSON and derives required IDs directly from exported runtime owners: UnitSpecs plus Nomadic Host, BuildingSpecs, TempleGods, CulturePrayers[MVPCulture], and fresh migrated goods with explicit commodity/service-role distinctions above. Terrain constants lack an iterable exported owner: use Go AST extraction of typed Terrain constants, or first add a tiny non-mechanical iterable catalogue approved by the parent. Require each current entity exactly once; reject retired aliases as independent entities; future articles remain separately tagged and cannot satisfy current entity coverage.

Generate numeric reference data through the same authoritative projections used by existing unit/building catalogue handlers, plus PrayerSpecs and existing terrain/economy owners. Reuse or extract projection assembly; do not copy costs or formulas into a new wiki owner. Article tokens can reference generated fields (`kind/id/field`) and fail on missing fields. Resolve recruitment totals through batch sizes and building upgrades through LevelledSpec, not raw map arithmetic. Clock/arrival predictions remain contextual live UI data, not static balance prose. No new endpoint is required for an initial build-time snapshot.

Concept coverage needs an explicit curated registry with source references: current code has no finite catalogue for concepts such as catchment, kharis, game days or upkeep. The capability list and notification kinds can provide additional discoverability guards but cannot prove all concepts covered. Future Oikos material requires vault/canon decisions and exact future marking from the order, not inference from dormant kingdom code.

Open canon question is tracked separately by the parent: force-mission provisions from support city versus freight provisions from physical departure port. Inventory does not resolve or change that distinction. No further mechanics claims, future goods mechanics, or global numeric export contract were approved in this checkpoint.
