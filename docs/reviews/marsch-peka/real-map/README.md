# Real-canvas run of the pick-a-hex mode

Rig: `rigg: commit 4385036 · migration 255/255 · värld f2e31309 · tick 0 · TICK_SECONDS=120 · 2026-11-11 22:27`

Script: `tools/hexpick_real_map_acceptance.py` (real `render/map.js` canvas, real pointer events, Firefox, isolated acceptance world).

| Step | Expected | Observed | Result | Shot | Evidence |
|---|---|---|---|---|---|
| desktop March pressed: bar shown, no drawer/panel visible | bar visible, panel list empty | bar="March 1st Spearmen of Amnissos: tap the destination Cities Cancel" panels=[] | PASS | R1-march-pressed-bar-only-desktop.png |  |
| desktop bar height | <= 64 px | 36.0 px | PASS |  |  |
| desktop canvas receives input | canvas not inert | inert=False | PASS |  |  |
| desktop nothing covers the canvas | elementFromPoint on map = hex-canvas | hex-canvas | PASS |  |  |
| desktop Cities shortcut sheet | sheet lists known cities, <= 40% height, no panel visible | "Amnissos — your city" h=38/800 panels=[] | PASS | R4c-cities-sheet-desktop.png |  |
| desktop drag pans the map and selects nothing | camera moved, no selection, no Use button | cam {'x': 132, 'y': -3145}->{'x': 132, 'y': -3075} sel=None | PASS |  |  |
| desktop tap hex A selects and outlines it | sel=(1,44), bar "(q,r) … Use this hex" | sel={'q': 1, 'r': 44} bar="March 1st Spearmen of Amnissos: tap the destination (1,44) — Use this hex Cities Cancel" | PASS | R2-hex-selected-desktop.png |  |
| desktop tap hex B moves the selection | sel=(2,43) | sel={'q': 2, 'r': 43} bar="March 1st Spearmen of Amnissos: tap the destination (2,43) — Use this hex Cities Cancel" | PASS | R3-selection-moved-desktop.png |  |
| desktop Use this hex opens the real march menu for that hex, unit checked | menu for (2,43), exactly one unit checked | #mctx-name="Olive Grove (2,43)" checked=[True] body.hex-picking=False | PASS | R4-march-menu-open-desktop.png |  |
| desktop menu does not cover the chosen hex | hex point outside menu rect | menu=300,270,210x522 hex=264,322 | PASS |  |  |
| desktop march sent: DB target is the picked hex | unit target=(2,43) | ['efd6ed70-6d7c-4751-9da2-12293ede8106/marching/2,43'] | PASS |  | units.target_q/target_r |
| desktop Redirect pressed: bar shown, no panel visible | bar, empty panel list | bar="Redirect 1st Spearmen of Amnissos: tap the new destination Cancel" panels=[] | PASS | R12-redirect-pressed-desktop.png |  |
| desktop Redirect: tap selects | sel=(5,42) | sel={'q': 5, 'r': 42} bar="Redirect 1st Spearmen of Amnissos: tap the new destination (5,42) — Use this hex Cancel" | PASS |  |  |
| desktop back on the card: "Redirect to (q,r) … Send order" | text contains Redirect to (5,42) and Send order | "Redirect to (5,42) — a messenger carries the order. Send order → · Change" drawer open=1 | PASS | R13-redirect-card-send-desktop.png |  |
| desktop Redirect order exists server-side | a messenger order carrying (5,42) | order/outbound/{"verb": "redirect", "recall": {"Mode": "redirect", "UnitID": "efd6ed70-6d7c-4751-9da2-12293ede8106", "WorldID": "f2e31309-d198-4c7d-ad58-643d868a2689", "NewTargetQ": 5, "NewTargetR": 42}, "unit_id": "efd6ed70-6d7c-4751-9da2-12293ede8106", "world_id": "f2e31309-d198-4c7d-ad58-643d868a | PASS |  | messengers.order_payload |
| desktop page status line | order sent line, no error | Redirect order sent — Runner following the unit — next meeting ≈ today 22:28. The order applies when they meet. | PASS |  |  |
| desktop Cancel restores the drawer exactly | drawer open, bar gone, no march menu, not picking | {'open': True, 'bar': False, 'ctx': 'none', 'pick': False} | PASS | R14-after-cancel-desktop.png |  |
| desktop Escape restores the drawer exactly | drawer open, bar gone, no march menu, not picking | {'open': True, 'bar': False, 'ctx': 'none', 'pick': False} | PASS | R15-after-escape-desktop.png |  |
| desktop right-click does nothing while picking | no march menu, no selection, still picking | march-ctx=none sel=None picking=True | PASS |  |  |
| desktop long-press does nothing while picking | no march menu, no selection, still picking | march-ctx=none sel=None picking=True | PASS |  |  |
| desktop after the pick a normal tap opens the hex panel | inspect panel visible | ['inspect-panel'] | PASS | R16-normal-tap-hex-panel-desktop.png |  |
| desktop Kult: Wake the Storm "Pick on the map" | reachable only with a temple + that prayer | button count=0 (not reachable in this world without a temple chain: not driven) | PASS | R17-kult-drawer-desktop.png | not driven |
| desktop no page errors | none | [] | PASS |  |  |
| mobile March pressed: bar shown, no drawer/panel visible | bar visible, panel list empty | bar="March 2nd Spearmen of Amnissos: tap the destination Cities Cancel" panels=[] | PASS | R5-march-pressed-bar-only-mobile.png |  |
| mobile bar height | <= 64 px | 52.8 px | PASS |  |  |
| mobile canvas receives input | canvas not inert | inert=False | PASS |  |  |
| mobile nothing covers the canvas | elementFromPoint on map = hex-canvas | hex-canvas | PASS |  |  |
| mobile Cities shortcut sheet | sheet lists known cities, <= 40% height, no panel visible | "Amnissos — your city" h=38/844 panels=[] | PASS | R8c-cities-sheet-mobile.png |  |
| mobile drag pans the map and selects nothing | camera moved, no selection, no Use button | cam {'x': 63, 'y': -3140}->{'x': 132, 'y': -3090} sel=None | PASS |  |  |
| mobile tap hex A selects and outlines it | sel=(2,47), bar "(q,r) … Use this hex" | sel={'q': 2, 'r': 47} bar="March 2nd Spearmen of Amnissos: tap the destination (2,47) — Use this hex Cities Cancel" | PASS | R6-hex-selected-mobile.png |  |
| mobile tap hex B moves the selection | sel=(3,46) | sel={'q': 3, 'r': 46} bar="March 2nd Spearmen of Amnissos: tap the destination (3,46) — Use this hex Cities Cancel" | PASS | R7-selection-moved-mobile.png |  |
| mobile Use this hex opens the real march menu for that hex, unit checked | menu for (3,46), exactly one unit checked | #mctx-name="Hills (3,46)" checked=[True] body.hex-picking=False | PASS | R8-march-menu-open-mobile.png |  |
| mobile march sent: DB target is the picked hex | unit target=(3,46) | ['efd6ed70-6d7c-4751-9da2-12293ede8106/marching/2,43', '26b4add1-9a7a-403a-af00-c9904effd560/marching/3,46'] | PASS |  | units.target_q/target_r |
| mobile no page errors | none | [] | PASS |  |  |

## Findings
- FIXED (commit "Cancel in the War March pick puts the War drawer back"): `unitMarch` closed the War drawer and never reopened it, so Cancel/Escape left the bare map (Redirect already kept its drawer). `chooseMarchTarget` now returns the hex or `null`; `unitMarch` reopens the drawer on `null`. Rows "Cancel/Escape restores the drawer" are PASS after the fix (they were FAIL on the first real-canvas run, rig commit 4780611).
- NOT FIXED (CSS, design): while picking, the one-line bar covers the top zoom buttons (the "+" at 1280x800, "+" and "−" at 390x844 are under the bar), although the picking note says zoom buttons stay. Wheel/pinch zoom still works.
- Not driven: Kult -> Wake the Storm -> "Pick on the map" (no temple chain in a fresh world; button absent). Cities shortcut: only the player's own capital is known, so the sheet lists just "Amnissos — your city" (shown, R4c/R8c); a rival city was not available (one real player).
- The script is not re-runnable on a used world (it asserts the units are in garrison); run `tools/acceptance.sh reset` first.
