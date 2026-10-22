# ORDER 14: client surface evidence

No production JavaScript, CSS or markup changed. The existing placement UI displays server capacities and marginal yields. BILD is therefore not triggered by this client diff; article readability is TEXT for playtest.

| Surface | Consumer and result |
|---|---|
| Temenos | economy owner and server tests: main implementation report. |
| Keryx | `server/cmd/keryx/cmd_city.go` fetches placement-options; `cmd_gubbe.go` `goodCell` displays server placed/cap and marginal_yield. `place` submits the selected good and hex to the server. No client fishing formula exists. |
| Megaron | `web/static/js/megaron/ui/citygrid.js` `goodRowHTML` uses cap, placed, rate_per_tick and marginal_yield directly. cap=0 disables +1 and omits Fill. Missing fish option results in no fish row. Citygrid never reconstructs land neighbours. |
| Codex | New Fishing and Harbours articles, linked from Food and each other; Buildings now states harbour scope includes deep water and its four places apply per water hex. |

`render/map.js` terrain-only Produces labels remain unchanged. They identify a potential good, rather than promising a worker slot. Numerical staffing capacity is shown in City placement options; no neighbour terrain or new map field is sent by this client change.

Verification: `node --test web/static/js/megaron/ui/citygrid.test.mjs web/static/js/megaron/ui/codex.test.mjs` — 32/32 green. The added renderer fixture checks cap 0 is locked and 2/3/4/6/7 places retain server numbers and 5.1 per-day marginal yield. It tests the existing UI contract; persistent end-to-end production evidence belongs to the server test package.
