# Native dispatch + omen evidence

Final run: `c4a66c74+dispatch-omen`, migration 215. `proof.json` records unchanged production-source SHA-256 values, three real maintained-temple omen rows, no browser errors, and fourteen screenshots (1280×900 and 390×844). `../bild.log` ends PASS. Owned server, PostgreSQL and Redis were stopped after the run.

The rig registers, joins and founds Lato through real APIs. Its explicit local fixtures add own noncapital Petras, terrain, temple level 3, shipyard/harbour, resources and initial standing 95. Real placement API calls staff temple/shipyard; real build/recruit/standing-order/rite APIs and workers produce the dispatches. The own and foreign transport rumor rows are explicitly labelled fixtures; omens come from actual temple maintenance. `maintained-db.json` records fed temple and standing gain; `omen-db.json` records true observed facts, and `gossip-api.json` records API `omen: true`.

Every browser path uses actual Notifications rows and normal drawer toggling. No mocked responses or DOM replacement. After each destination action the rig asserts exactly one open drawer.

- `omen-feed-*`: real ✦ Omen, Lumbermill with city/hex, Emporos with city. Own transport fixture absent, foreign transport fixture present (also API assertions).
- `build-dispatch-*` → `staff-selected-*`: Staff Lumbermill opens Petras and the exact API-confirmed field ordinal at (16,23), with staffing controls.
- `train-dispatch-*` → `unit-focused-*`: Show the ship opens War and highlights the actual newly completed Emporos.
- `cult-selected-*`: actual RiteAnswered opens Cult in Petras.
- `route-selected-*`: actual standing delivery dispatch opens Economy Automation and highlights its existing Lato→Petras route.

Goods and cargo destination mappings have focused JavaScript tests; this browser rig does not claim separate end-to-end Goods/cargo click coverage. The staff panel is selected and actionable; the screenshot phase does not submit an additional worker placement.
