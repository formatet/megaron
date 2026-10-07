# Expedition clients

Keryx, the map order menu, War → Army and Codex now describe exploration as an area expedition with a duration in game days. Duration bounds, default and radius come from Temenos’s `expedition_rules`; no client tunables are maintained. CLI `--ticks` is explore-only and passes through both POST dispatch and GET preview. Roster/dispatch receipts and Army cards show authoritative mission ticks; return reasons are readable.

- `go test ./cmd/keryx -count=1`: passed. New `TestExploreTicksOrderAndPreview` covers unit command and march alias, GET/POST, duration and receipt. `TestExploreTicksRejectOtherIntents` and `TestExpeditionMissionLifecycleText` passed.
- Focused expedition, march menu and preview JS tests: 27 passed. Includes changed server rules, boundaries, invalid/unavailable duration, outbound/homeward mission strings, existing per-unit forecasts and stale response protection.
- `python3 tools/expedition_acceptance.py`: two clean Chromium runs of actual modules with authenticated API fixtures. Controls start at server defaults, edited 12-day duration reaches preview GET and order POST from both entries, mission row is readable, mixed selections explicitly distinguish expedition starts from redirects, redirect-only selections hide duration. Desktop and 390px controls fit without horizontal overflow. [Run 1](client-proof.json), [run 2](client-proof-run2.json).
- Existing `tools/march_preview_acceptance.py` remains green, with fixture rules added to match the current unit-list response.
- Mutation: dropping `ticks` from the CLI shared request body makes all four alias/unit + preview/dispatch cases fail; restoring it passes the full CLI package. [Mutation log](client-mutation.txt).

1:1 captures: [map desktop](expedition-map-desktop.png), [map 390px](expedition-map-mobile.png), [Army desktop](expedition-war-desktop.png), [Army 390px](expedition-war-mobile.png), [homeward mission](expedition-mission.png). Inspected at native size: readable, no clipping.

Limits: browser fixture proof does not claim live gameplay or process provenance. Root integration owns the real register/join/found/expedition lifecycle and full fresh-database checks. Before dispatch the summary promises only the selected area, duration and relative turn rule; absolute days come from Temenos when the order starts. Unknown exploration routes retain unavailable arrival forecasts. Text comprehension remains for playtest.
