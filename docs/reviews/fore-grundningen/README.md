# Before the first city (order-fore-grundningen-20261113)

Source: `docs/reviews/forsta-timmen/README.md` §2-§3 (main checkout). Pictures: Firefox, isolated acceptance world,
`python3 tools/fore_bild.py after /tmp/claude-1000/kx-fg2.json` (1280x900 and 390x844). Players registered, none founded.
`before-*` were taken at master + part 1 only (so they show the old forecast, dispatch (old stored text), drawers and
Report button); `after-*` on the final code. One commit per part (`git log fore-grundningen`).

| Part | What | Test | Pictures |
|---|---|---|---|
| 1 Legend | Legend button (▤) in the map button column opens a window (`openMobilePanel`) listing deposits (from `DEPOSIT_KINDS` in `render/map.js`), cities, Host, army, foreign army, ship, caravan, messenger, workers, storm. Every symbol is drawn by the map's own function (`drawDepositIcons`, `drawProvinceIcon`, `drawActor`, `drawWorkerMarker`, `drawStorm`). ♫ and ⚒ got plain titles and `aria-label`s: ⚒ shows what each of your cities is doing (building, training, idle), which is what it really does | `render/legend.test.mjs` | after-legend-1280/390 (before: playtest shots 0036-0037, 0044) |
| 2 Forecast | Rows are whole numbers from the existing `/colonize-preview`: Food (grain + fish made, eaten), Timber, Stone (about N a game day), Copper/Tin/Silver (hexes in the known catchment, or none seen), Ground (commonest terrain), Ships. "Based on N of M hexes known" and coordinates kept. No server change. Keryx already prints numbers; unchanged | `ui/forecast_rows.test.mjs`, `host_simple.test.mjs` | before/after-host-panel |
| 3 Dispatch | New payload: `paragraphs` = [who you are, game-day line] + `steps` (five lines), 54-60 words. Stored dispatches keep their text and render as before. Every rule the old text carried was already in the Codex except the Host's 1,200 grain, now in `horde`; `welcome` lists where the rest lives. Web renders an `<ol>`, keryx prints numbered lines | `opening_dispatch_test.go` (word count, 5 steps, game-day line), keryx + web tests | before/after-dispatch |
| 4 Words | Empty City/Economy/War-Recruit say "Click your Host on the map, then press Found the metropolis here." (only while a Host exists). Bottom-bar REPORT and drawer title are FEEDBACK (ids, `keryx report`, article id kept; Codex and alpha page follow). Top-left name has a tooltip and the Account window a line: "Your Wanax's name. You signed in as X." (did not exist). Glosses: world entry card ("Wanaxes (rulers)", "metropolis (first city)") and Host panel (one line + "? Codex" link). World title uses `--text` | `no_city.test.mjs`, account, host tests | before/after-drawer-*, account, join |

## Not done / not tried
- No "before" picture of the entry card or old legend: the card needs an unjoined account on old code; playtest shot 0010 is the before.
- "Map banner" gloss: the top-left name has the tooltip (hover only); the dispatch chip text has no jargon. The banner itself is not glossed.
- Legend shows every symbol always, not only those the player has met. Storm sample is a single live hex without animation.
- The legend glyph (▤) was chosen to differ from the Codex "?"; Timothy may prefer another.
- `water_test.go` allowlist still names `opening_dispatch.go` (now harmless, entry unused); left alone.
- Dispatch refusal texts and the Codex wording of Feedback were not judged by a stranger (TEXT, playtest).
