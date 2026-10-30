# Lake names — insjöar slice 3 (Claude 2026-10-30)

Order `.agents/order-insjoar-20261030.md` slice 3. Canon: vault `megaron_inventering_insjoar_20261030.md` §Kanonfrågor 4–5
(Timothy 2026-10-30): every lake is named, even one hex; the right goes to the first to sail there; the epitaph says
"on the shore of <lake>" once the lake has a name.

## Contract

| Rule | Where |
|---|---|
| `lake` is a place kind: each connected lake component, any size | `world.Places` (spec min 1), `world.IsPlaceKind` |
| DB accepts it | mig 233 widens `place_names_kind_check` |
| Naming right = first entered lake hex of a frozen journey — only ships can enter, so the first ship | unchanged generic scan `combat/place_reach.go` |
| Naming API accepts `lake` | `place_names.go` via `world.IsPlaceKind` |
| Epitaph: metropolis beside a **named** lake and no Thalassa neighbour → "X rose on the shore of <name>." Unnamed lake or sea neighbour → old line | `web.go withLakeShore` (read at render time) |

Four surfaces: temenos (above) · keryx (`places` label, `places name lake`) · megaron (`PLACE_LABELS`, notices,
"Lake" map fallback, tell-place parser, Umami path redaction) · Codex (`lake.md`, `naming-the-land.md`).

## Proof

Fresh DB on mig 233: `TestPlacesLakeEverySize`, `TestPlaceReachShipFirstOnLakeClaims`, `TestPlaceLakeCanBeNamed`,
`TestEpitaphLines_LakeCityRoseOnTheLakeShore` green; web 880/0.

Mutants (4/4 killed):
- M1 drop the lake spec in `Places` → `places_test.go:56`, `place_reach_test.go:603`
- M2 old CHECK in mig 233 → `place_names_test.go:250` (check constraint)
- M3 drop the Thalassa guard in `withLakeShore` → `epitaph_test.go:207`
- M4 old kind list in `placeIDs` → `place_names_test.go:253` (HTTP 400)

## Not done / open

- Colonies keep "Founded X." — the canon spoke of the founding line only.
- An unnamed lake city still reads "the Thalassa" (canon: switch when named). Flag to Timothy if it grates.
- TEXT in playtest: the notice "first to reach this lake — you may name it".
