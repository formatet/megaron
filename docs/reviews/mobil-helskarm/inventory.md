# ORDER 11 — inventory before code

Base d6b0032c. Existing mobile breakpoint: max-width 480px (CSS drawer/Codex/nav rules).

| Surface | Current stacking/anchor | Closing |
|---|---|---|
| City, War, Diplomacy, Economy, Kult, Report, Notifications drawers | fixed right, top34/bottom34, z200; mobile bottom68 | header X, own nav toggle, Escape via main.js |
| Inspect (terrain, rural, foreign city, Host) | absolute right inside game-map, full map height, width220, z15 | header X / closeInspect resets selected hex |
| March, including city destination chooser | fixed at click, width210, z300, scrolling whole box | header X, Escape, outside click |
| Dispatch/individual notification (chip or archive row, same window) | fixed inset0 overlay z500; box max420, starts12vh | header X, backdrop, Escape |
| Account | same z500 overlay; box starts12dvh, own scrolling body | header X, backdrop, Escape |
| Search | fixed inset0 overlay z500; box max540, starts8vh, results max50vh | ESC button, backdrop, Escape; selecting target |
| Codex | fixed left top34/bottom34 width440 z210; mobile bottom68 | header X / closeCodex, back within article history |

Tooltip is pointer-events:none, z400 in map inline CSS (shared CSS also has older z200); transient hover, not an interactive window. Suppress while mobile panel open. Map zoom tools, activity badges, topbar chips/calendar and navigation are persistent chrome, not windows. Lawagetas brief and city detail are inline inside drawers, not separate overlays.

Archived observation confirmed (megaron_todo_arkiv_20260923 ~2561): CSS drawer z200 covers inspect z15; the current main.js openDrawer already closes inspect and resets selectedHex (the archived missing-cleanup issue was fixed). March z300 still stays above drawers. Codex z210 can coexist with desktop drawers intentionally. Preserve desktop coexistence.

Proposed mobile contract: one shared panel coordinator at existing 480px breakpoint; registered open/close lifecycle for the seven surface types above; new panel closes previous via its existing cleanup callback, preventing hidden click targets and stale drawer state. Shared fullscreen class, safe-area padding, fixed header and internal scrolling; map made inert and touch/pointer input blocked until last panel closes. CSS mobile-only, desktop geometry unchanged. Closing returns map input. Screen Codex article describes right-side inspect and needs mobile qualification. Server/CLI/API untouched.

Proof: JS lifecycle tests, physical coordinator mutation, Firefox 390 has_touch tap for real city/hex/march-with-city/drawer/dispatch, two clean runs, plus desktop1000 baseline comparison; full fresh Go once on57900 + JS. BILD before merge.
