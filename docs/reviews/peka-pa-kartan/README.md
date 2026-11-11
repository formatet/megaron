# Point at the map (web only, branch peka-kartan)

Order: `.agents/order-peka-pa-kartan-20261111.md`. No server or keryx change (`git diff --stat master -- server` is empty).
Inventory: `inventory.md`. BILD script: `tools/peka_kartan_bild.py` (Firefox; real hexpick/war/kult/force_card/megaron.css and the
map.html chrome; the canvas is a stand-in grid wired to the real `pickTap`, since `render/map.js` needs a live world).

## What it does
- `ui/hexpick.js`: `pickHex({prompt, accept})` -> `{q,r}` or `null`. `render/map.js` hands plain taps to `pickTap` and ignores
  right-click/long-press while picking; Escape cancels (main.js). First tap selects (outlined, bar shows "(q,r) Name — Use this hex"),
  another tap moves it, the button confirms. `accept` may return a reason (button absent). A second pick cancels the first.
- Redirect (War force card): pressing it starts the pick at once; back on the card: "Redirect to (q,r) — a messenger carries the
  order. Send order -> · Change". Same POST body. "or type coordinates" stays; inputs start empty; empty Q or R is refused with "Type both Q and R."
- Wake the Storm: "Pick on the map" next to the Sea hex field; accepts seen sea only ("Not a sea hex", "You have not seen this ground");
  fills "q,r"; field stays editable; rite body unchanged.
- Codex: `wake-the-storm.md`, `runners.md` (stayed inside the 1490 cap); `codex_facts.py --check` clean.

## What hides and what stays while picking
Hidden (CSS `body.hex-picking`, visibility so typed text, scroll and tabs survive; nothing is closed): all drawers, inspect panel, march
menu, dispatch/account windows, search, Codex panel, map dim. Inert and dimmed: bottom nav, notif/search/Codex top-bar buttons.
Stays: top bar, zoom buttons, the map (pan/zoom as usual). The phone's "map is inert while a panel is open" rule is lifted while
picking (`mobile_panels.js`), and re-synced afterwards.

## Pictures (Firefox; 1280 x 800 = K1-K5, 390 x 844 = K6-K10)
K1/K6 Redirect pressed: bar, no drawer · K2/K7 hex selected, "Use this hex" · K3/K8 back on the card with Send order ·
K4/K9 refused hex (not sea) · K5/K10 Wake the Storm field filled. The script asserts: at 390 no drawer/panel is visible while picking,
the canvas is not inert, the bar is <= 64 px (measured 36 px desktop, 39.6 px at 390), the Send body is exactly
`{target_q,target_r}`, empty Q/R posts nothing, the rite field holds `3,-1`.

## Count (390 px)
War · Redirect · tap hex · Use this hex · Send order = 5 taps, whole map visible while choosing.

## Part D: what covers the map at 390 x 844 (report only, the next slice)
Measured with the real `mobile_panels.js` + megaron.css in Firefox: surface rect vs canvas rect, `canvas.inert`.
| Code | Flow | On screen over the map | Target visible? |
|---|---|---|---|
| P1 | March here / march menu (tap or right-click) | `#march-ctx` full screen (100%), map inert | No |
| P2 | Colonize (checkbox in the march menu) | same march menu, 100% | No |
| P3 | "Show ..." from a dispatch | dispatch window closes, then the inspect panel opens full screen (100%) | No |
| P4 | Clicking a chip | dispatch window full screen (100%) | No (it is the news, not a map) |
| P5 | Send goods -> | Economy drawer full screen (100%) | No map needed |
| P6 | Build pick mode (city grid) | City drawer full screen; the pick is on the in-drawer grid (the earlier bygg-hexklick H8/H9) | Grid yes, map no |
| P7 | War -> March (`chooseMarchTarget`, hex panel Redirect/March) | march menu full screen saying "Choose a destination on the map." with the map inert | **No: the instruction cannot be followed on a phone** |
Pictures: `P1`..`P7-covers-map-mobile.png` (fixture content is empty chrome; the covering is the finding). Raw numbers: `part_d.json`.
Main finding: P7 and P1 are the same fault as Redirect had. Next slice: War -> March and the hex panel's March/Redirect through `pickHex`.
Also P3: tapping any hex on a phone opens a full-screen inspect panel, so "tap the map" always ends covering it.

## Removed / changed
Removed the "Right-click the new destination on the map" instruction on the force card. Typed-coordinate defaults 0 -> empty
(`war_cards.test` still passes: it only checks the input ids). The hex panel's own Redirect (map.js `data-hex-redirect`) still uses
the march menu: untouched.

## Not done / caveats
- `render/map.js` hook (`pickActive`/`pickTap` in tap and orders, outline of `State.pickHexSel`) is three small edits not covered by a
  node test (map.js cannot load in node); the BILD drives the real `pickTap` from a stand-in canvas.
- Red tests: the state machine tests were committed together with its module; the Redirect/Wake tests were red first (verified).
- Tests: 969 pass (955 on master + 14 new).
