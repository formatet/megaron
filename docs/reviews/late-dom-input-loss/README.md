# Late DOM replies erasing player input

Codex (`2816f374`) reproduced thirteen cases where a slower HTTP reply rebuilt DOM the
player was already typing in. Claude fixed them (Codex was idle from 04:27) client-side only:

- **Same root, newer-wins sequence guard** (`cityLoadSeq`, `warLoadSeq`, `kultLoadSeq`,
  `transferGoodsSeq`, `automationLoadSeq`, `buildTypeSeq`): an older reply no longer fills nodes a
  newer load built.
- **`innerHTML +=` after `await`** (City Garrison, War Recruit) -> `insertAdjacentHTML`.
- **Build picker**: a hex chosen while a refresh is pending survives it (`preferredHex`).
- **Silent grid refresh** restores the live selection (WeakMap), not the one it started with.
- **Correspondence**: `snapshotDrafts`/`restoreDrafts` keep text, selection, focus, trade fields,
  buy/sell choice and open trade details across a thread re-render.
- **Search**: the late message index keeps an arrow-selected row.

Probe fixes (not product code): focus assertions needed the hidden tab / closed `<details>` opened
first, and the fixture lacked `/api/v1/goods` and an offer-good choice.

Evidence: `after/results.json` 13/13 pass; 438 JS tests; three mutations (city load guard,
`restoreDrafts`, grid live selection) each turn the named probe red. Isolated HTTP fixtures — not a
real-game/FOW proof (see `tools/late_dom_probe.py`). Run: `python3 tools/late_dom_probe.py OUT`.
