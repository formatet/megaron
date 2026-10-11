# Slice B: markraster worker

Locked baseline: `547fd79e` (master after A and river names, contains `8a34b5d6`).
Branch: `codex/markraster-worker`. Gate: proves that map baking stops freezing the
player's main thread. No verb, server, CLI or Codex rule changes.

`markraster.js` owns the existing terrain palette, geometry/noise and field drawing
plus `bakeChunk`. It receives an explicit scene and canvas factory, with private
instance caches; it imports neither game State nor DOM. `map.js` uses that same
renderer for live geometry and the existing synchronous budget fallback.
`markraster_worker.js` uses OffscreenCanvas and transfers ImageBitmap ground/memory
masks plus the code/surf buffers. There is one bake in flight; viewport jobs precede
the prefetch ring, and camera changes replace queued jobs. New scene generations
discard late results, close their bitmaps and invalidate the world cache. Existing
chunk signatures still reuse unchanged chunks on tile/city refreshes.

Worker 2D/bitmap support is probed inside the worker. Missing APIs, constructor or
runtime failure, message failure and startup timeout select the original budget
path. Initial missing chunks arrive progressively; existing chunks remain visible
during refresh. Final pixels and all visual rules are unchanged.

## Pixel contract

Firefox first, then Chromium; DPR 1. Forest, relief and river at zoom 1.0, same
frozen rig frame and cameras as the baseline. World viewport 1600×900, other
viewports 1040×900. Screenshots crop `#map-root`.

**14/14 comparisons have exactly 0 changed RGB pixels**: world, forest, relief,
cities, river, world minzoom and world FOW, in each engine. Each comparison has a
`<engine>-<fixture>-before.png` and matching `after.png`; [after.json](after.json)
contains counts. [Firefox river after](firefox-river-after.png) is the 1:1 example.
The baseline source is fetched by `git show 547fd79e:…/map.js` and routed into the
browser; it cannot be contaminated by edits in this worktree.

## Main-thread blocking

Same machine and fixtures, before then after serially. Longest synchronous
`render()` task across the first frame and all subsequent settling frames; rapid
zoom-out changes the camera from 0.55 to 0.3 without warming the new chunks.
A 5 ms main-thread timer independently records its maximum lateness. Firefox's
clock quantizes the values. These are measurements, not a frame-time SLA.

| Engine / trigger | Longest render block, before → after (ms) | Max timer lateness, before → after (ms) |
| --- | ---: | ---: |
| Firefox first world view (0.55) | 850 → 47 | 849 → 51 |
| Firefox first world view (0.3) | 1551 → 76 | 1550 → 75 |
| Firefox rapid zoom-out | 196 → 43 | 191 → 41 |
| Chromium first world view (0.55) | 638.3 → 51 | 650.4 → 55.2 |
| Chromium first world view (0.3) | 1185 → 68.4 | 1194.7 → 70.5 |
| Chromium rapid zoom-out | 167.8 → 42.5 | 165.7 → 40.7 |

The total bake work is still present in the worker. Live overlays, culling and
chunk signatures remain on the main thread; minzoom still has a 68–76 ms render
block. `bakeMs` measures aggregate bake CPU time, not time to the completed view.
Raw measurements: [before.json](before.json), [after.json](after.json).
Browser versions, fixture hashes and production-source hashes: [metadata.json](metadata.json).

## Lifecycle, fallback and mutation checks

[Lifecycle results](lifecycle.json): real worker first-view pending work is
superseded by new world salt, unknown fog, remembered river mask and city-driven
cultivation. Its final image equals the no-OffscreenCanvas, constructor-failure
and runtime-failure budget paths, with **0 changed pixels in all six comparisons**.
An equal-content tile/city refresh adds **0 bakes** in all eight scenarios.
The old `markraster=0` relief renderer also runs. No console or page errors were reported.

[567 JS tests pass](js.log), versus 562 on the immutable baseline. Five meaningful
new tests cover queue priority/single flight, stale scenes/resource release,
failure fallback, constructor failure and independent renderer scenes.
[Five real production mutations](mutations.json) produce named reds:

- Bypass generation gate → `late river/FOW/world result…`.
- Keep obsolete queued camera jobs → `worker queue keeps only one bake…`.
- Skip stale bitmap close → `late river/FOW/world result…`.
- Disable worker → `supported browser must bake in worker`.
- Remove warp → `bitidentical forest pixel contract` (73,074 changed pixels).

Every mutation restores its edited bytes in `finally`; [restored.log](restored.log)
is green. The existing A mutation tool now targets the extracted renderer. Its three river
mutations also produce named reds and restore green in [river-mutations/](river-mutations/).

Reproduce from this worktree:

```sh
python3 -m http.server 8750 --directory web
# In another terminal:
python3 tools/markraster_worker.py --mode before
python3 tools/markraster_worker.py --mode after
python3 tools/markraster_worker_lifecycle.py
python3 tools/markraster_worker_mutation.py
python3 tools/markraster_flod_mutation.py --out docs/reviews/markraster-worker/river-mutations
node --test web/static/js/megaron/**/*.test.mjs
```

Synthetic rigs only. Slice C / real-game acceptance has not started. No merge,
push or deploy by Codex; hand over hash and STOPP to Claude.
