# River terrain row

Base `3e25da49`, branch `codex/flod-terrangrad`. Hex and rural panels keep
`Ford of Eurotas` (or River / Delta) in the title and now show only `River ford`,
`River` or `River delta` in TERRAIN. Unknown-name forms use the same labels.
The shared field renderer fixes both panels. No river naming or FOW rule changes.

Keryx audit found one second copy in `city`: a building suffix repeated the named
terrain already printed in the main catchment column. That suffix now uses the
ordinary terrain key; river name, building level and coordinates remain intact.
`map` and `place` already print the name once per row and need no change. JSON
keeps the separate terrain and river_name fields.

Contract before fix: [JS red](contract-red-js.log), [Keryx red](contract-red-keryx.log).
[567 JS tests](js.log) and [full Keryx package](keryx.log) pass. Existing real
panel-input test now covers all three forms in hex and rural panels. New real
`city` command test catches duplicate names beside a production building.
Two production mutations restore each bug: [terrain row](mutation-terrain-row.log),
[Keryx row](mutation-keryx-row.log); original bytes restored.

Firefox 1280×900: six before/after panel pairs in this directory, covering all
three forms × hex/rural. [Ford after](firefox-1280-3-hex.png),
[rural ford after](firefox-1280-3-rural.png). [Browser assertions](browser.json).
Actual map shell, renderer and pointer handler, saved real handler river payload
from the prior names proof; surrounding plains/cities/rural buildings are synthetic.
Worker settling completes before every screenshot. No page/console errors.

Reproduce: `python3 tools/flod_terrangrad_browser.py --baseline`, then without
`--baseline`; `python3 tools/flod_terrangrad_mutation.py`. Full checks:
`node --test web/static/js/megaron/**/*.test.mjs`; `go -C server test ./cmd/keryx -count=1`.
Previous naming mutation tool follows the title now that TERRAIN is generic.

No merge, push or deploy by Codex. Separate slice C follows this handover.
