# Keryx notifications without raw JSON

Core `b2213ca2ab506f894cd811de0b713759a3696ac1`; base `fd76e664`, branch `codex/keryx-notiser-utan-json`. Scope: CLI rendering and coverage/proof; server, schema and gameplay unchanged. This proves readable feedback on the geography → shortage → bronze → elite chain. Independent TEXT judgment remains with review/playtest.

`notifications` now prints the unread/age/kind header followed by human detail. The same renderer serves `watch`; explicit routine-kind groups retain ×N and show the latest readable detail. Filters, unread counts, critical-first priority and mark-read behavior retain their existing paths. Raw response/payload remains available with `--json` and is checked without dropping unknown fields.

[Inventory](inventory.md): **78** kinds in the union of server producers and the Codex client's icon/text list; **68** have current server producer paths, with the others retained for archive readability. **42** previously lacked a detail row. The test discovers NotifyPlayer calls, notification INSERTs, constants, locals and forwarded kind parameters/closures from production Go ASTs; unresolved producer kinds fail. It extracts the exact same client cases as `codex.test.mjs`, rather than maintaining another fixed list. Persisted-delivery methods forward rows already covered by their INSERT producers.

New summaries name completions, arrivals, losses, orders, offers, trade, food support and conquest. Existing unit names formerly visible only inside JSON survive in ship repair/damage and homeward reports. Sender/opponent/besieger precede place when both occur; internal tick values are shown as days. Historical time fields remain readable. Granary releases name `per_good` food rather than assuming grain, and small positive amounts keep significant digits instead of becoming zero. ColonyFounded gets a founding row even in an old body without grain-balance fields.

## Actual rig list, before and after

Captured Eirene's actual GET `/api/v1/worlds/{world}/notifications`: **51 notifications**, unmodified response bytes in [rigg-notifications.json](rigg-notifications.json). Capture provenance: rigg: commit 07f5ae9 · migration 171/171 · värld 3b5fb9cd · tick 828 · TICK_SECONDS=6 · 2026-10-11 21:22. This is the existing acceptance rig, with server application `07f5ae9`, not a freshly deployed server from this branch. Only read requests were made; no game state, container setup or account state was changed.

To give both binaries identical input, [replay.py](replay.py) serves the frozen real response on loopback. Before binary built from pristine fd76e664; after binary built from this branch. Both run `notifications --exclude none` so the server snapshot is shown in full. This is a replay of actual rig data, not fabricated notifications. The new binary was additionally run directly against the live rig: [output](rigg-after-live.txt), [provenance](rigg-live-provenance.txt).

Before (actual rows):

```text
*[Sun Oct 11 20:06]  UnitArrived           {"q":12,"r":80,"name":"1st Spearmen of Mycenae","status":"garrison","unit_id":"d86cebbc-10fb-47ed-9494-e0ffd62fd687"}
*[Sun Oct 11 20:00]  MetropolisFounded     {"q":12,"r":80,"name":"Mycenae","coastal":false,"grain_days":680043543732944900,"grain_ticks":680043543732944900,"known_hexes":19,"province_id":"e632c045-6ab5-4b1a-8714-efd25e0cab1d","grain_amount":1208,"grain_carried":1200,"settlement_id":"a455b324-b42f-4bef-8b8e-a36ac443b190","unknown_hexes":0,"silver_carried":462,"grain_net_per_tick":-0.0000000000000017763568394002505}
```

After (same rows):

```text
*[Sun Oct 11 20:06]  UnitArrived
      1st Spearmen of Mycenae arrived at (12, 80) — garrison
*[Sun Oct 11 20:00]  MetropolisFounded
      Your metropolis is founded: Mycenae — 19/19 catchment hexes known
```

Full [before output](rigg-before-human.txt) (12730 bytes) and [after output](rigg-after-human.txt) (5981 bytes). Both [before JSON](rigg-before-json.txt) and [after JSON](rigg-after-json.txt) parse equal to the complete original response, including every body field.

A pre-existing numerical artifact is visible in that real MetropolisFounded payload: grain net `-1.7763568394002505e-15` produces grain_days/grain_ticks about `6.8e17`. It is preserved in --json; the newly added metropolis summary follows the web's founding/name/catchment wording. This slice does not alter the server's grain calculation.

## Checks and mutations

- Fresh PostgreSQL 16, migration 171: baseline Keryx package green; source-derived coverage on baseline named 42 missing details red. [baseline log](baseline-go.log), [missing-detail log](baseline-coverage.log).
- [Coverage](coverage.log): iterate all 78 kinds through actual detail rendering, plus ordinary and live headers; require a nonempty non-JSON detail and no player-facing tick words. Command-level fixture verifies critical priority, routine grouping, no raw data and full JSON preservation. Semantic fixtures cover days and historical aliases, direction-aware offer refunds/arrival day, named unit loss/order, mixed fish/grain, fractional grain and person-before-place.
- [Fresh full Go suite](full-go.log): 25 tested packages green. `go vet ./cmd/keryx` and CLI build passed. [Codex list/article test](codex-test.log): 12 green. No web UI changes.
- [Mutation script](mutate.py) physically changes one thing at a time and restores bytes in finally. Removed TrainComplete handler → [named red](mutation-missing-kind.log); reintroduced ordinary raw body → [named red](mutation-raw-row.log); reintroduced grouped raw body → [named red](mutation-raw-group.log); added a real server NotifyPlayer producer `BrandNewNotice` without CLI detail → [named red](mutation-new-server-kind.log). [Restored package](mutation-restored.log) green.

Reproduction: build baseline and branch Keryx, run `python3 docs/reviews/keryx-notiser-utan-json/replay.py <before-binary> <after-binary>`; run `python3 docs/reviews/keryx-notiser-utan-json/mutate.py`; run `tools/gotest.sh`.
