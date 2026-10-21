# Explore refusal at a chosen fog hex

## Summary

The repeated Explore refusals for Talthybios and Pheidon in world `2178e795-1303-4cc3-865a-6386847d68d1` are real route refusals. A selected fog hex marks an area; it does not make that area accessible to the chosen unit. The branch makes that existing area-specific refusal and the help text explain the selected hex's role and the unit's route. No live game state was changed.

## Slice contract

| Field | Contract |
| --- | --- |
| Problem | A player repeatedly selects a visible fog frontier and receives only “none of the unseen ground ... can be reached.” |
| Player truth | The order explains that the selected fog hex is a search area, not a guaranteed route from the unit's position. |
| Invariant | An unseen land or water barrier is never bypassed. No order is sent by a preview. |
| Scope | Explore refusal in `combat`, existing Keryx help, map menu hint, Codex marching article. |
| Non-scope | The separate decision to send a future expedition first to the exact chosen hex; map generation and transport between disconnected regions. |
| Acceptance | Reproduce both cases from read-only live data; a disconnected target yields the route explanation; knowing an unrelated reachable hex does not change the reply. |
| Stop condition | The error would reveal hidden terrain information beyond the chosen area's existing FOW policy. |
| Proof | Live read-only SQL, fresh-DB domain test, mutation, full Go and Node suites, vet. |

## Baseline

Live world at tick 831 was inspected with `BEGIN READ ONLY` via PostgreSQL. Pheidon's spearman stands at `(0,41)`, and an Explore preview for `(3,31)` returned HTTP 422; the target is an unseen olive grove. A recursive traversal using the game's axial neighbors and land passability found 86 reachable land hexes, all 86 already in Pheidon's `player_scouted_tiles`. The target is in a separate land component. Talthybios's ship departs Pylos through `(39,28)`; it can reach 13 water hexes, all 13 known. The unseen deep-sea target `(39,39)` is in a separate water component. The live `commands.jsonl` records the same Explore refusals. These are correct refusals; accepting either order would contradict physical movement.

## Experiment and gates

The refusal still runs only after the selected area has unseen passable candidates but none yielded a route. It now explains that the selected hex defines a search area and does not grant passage. It makes no claim about unknown ground elsewhere. A fixture with an impassable water wall checks that the response is unchanged when an unrelated reachable hex changes from known to unseen. The first draft used a global movement-component scan; Claude rejected that stronger FOW inference, and it was removed before commit. Restoring the old generic error as a mutation made `TestExpedition_UnreachableChosenAreaExplainsWithoutGlobalClaim` fail on a fresh migration 195; restoring the new text made it green on a separate fresh database.

| Gate | Result |
| --- | --- |
| Code | Full Go suite green on fresh migration 195 before rebase (`internal/world` 156.869 s). After rebase onto master `e0304f5c`, the Explore and order-supersession integration tests were green together on another fresh migration 195; 692 Node tests and targeted vet green. |
| Semantic | No route or unit state changes; the existing API error path is shown in web and Keryx. |
| Visual | Text changes only; no layout or graphics change intended. |
| User | Read-only live commands and SQL establish the refusal; a local fixture tests the new response. |
| Drift | No merge, push, deploy, or live mutation. |

## Scope boundary

Timothy's separate decision to send a future expedition first to the exact chosen hex remains a later slice. It cannot make the two selected live targets reachable across disconnected terrain.
