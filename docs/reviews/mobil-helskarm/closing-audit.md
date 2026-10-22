# Registered-panel close audit

Search patterns across `web/static` JS and HTML: `classList.remove('open')`,
`classList.toggle('open')`, `style.display = 'none'` (both quote styles), plus
registered panel IDs and their inline handlers. Reviewed against d61992a5.

| Registered surface | Hiding writes and callers | Registry cleanup |
|---|---|---|
| Drawers (City/War/Diplomacy/Economy/Kult/Report/Notifications) | main.js closeDrawer; header X, toggleDrawer, Escape, replacement | closeMobilePanel before remove(open); city animation stops |
| Inspect (terrain/rural/foreign/Host) | render/map.js closeInspect; rural-city button; own-city branch in openInspect | All three hiding writes already call closeMobilePanel; city transitions then call openDrawer/closeInspect |
| March | marchctx.js closeMarchCtx; X, Escape, outside click, target change, send completion, map handoff | closeMobilePanel before display=none; unit/destination state reset |
| Dispatch | dispatch_window.js closeDispatchWindow; X, backdrop, Escape, action/navigation handoffs | closeMobilePanel before remove(open) |
| Account | account_window.js closeAccountWindow; X, backdrop, Escape, toggle, signout | closeMobilePanel before remove(open); generation invalidated and secret cleared |
| Search | search.js closeSearch; ESC button/toggle, f or slash when not typing, backdrop, Escape, result click/Enter/drawer handoff | d61992a5 already had `else { closeSearch(); }` after classList.toggle. Now explicit early closeSearch+return, followed by add(open) only for opening. |
| Codex | codex.js closeCodex; X, topbar toggle, Escape, replacement | closeMobilePanel before remove(open) |

Other matches hide child fields, tabs, badges, previews, tooltip or inline
content; none is a registered top-level panel. No additional missing registry
cleanup found. Desktop retains simultaneous panels intentionally.

Browser checks use actual modules/HTML and touch taps for headers, search ESC,
result and notifications. Also exercise search toggle/Escape/slash/backdrop,
March Escape/outside click, drawer/Codex/account toggles, account and dispatch
Escape/backdrop, and verify canvas.inert=false, no map blocked class and no
mobile-fullscreen element after closing. Fullscreen has no exposed backdrop;
those backdrop branches use DOM click dispatch explicitly, not a claimed tap.

`search_lifecycle.test.mjs` calls actual toggleSearch with a narrow DOM fixture.
`search-mutation.log`: rescued baseline GREEN; physical mutation removing
closeSearch from the closing branch RED; exact restored code GREEN. The
reported original search leak is therefore not reproduced and is not claimed
as a bug fixed by this continuation.
