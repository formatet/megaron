# Stable march quantity controls

Contract: proves orders/geography. Changing troop quantity must not move the menu
or its quantity input. Forecast content may grow inside the available viewport.
Scope: marchctx.js positioning, browser geometry proof with explicit API fixtures.
No order semantics, forecast formulas or visual family changes. Initial placement
may fit the loaded unit list; later forecast updates preserve that anchor.
Baseline: repositionMarchCtx reruns positionMarchCtx using current content height
on every asynchronous preview, so shorter/taller results change the top coordinate.

Validation: `python3 tools/march_position_check.py` RED before fix: quantity input jumped 191px (top342.7→533.7). GREEN after: menu top410, input top501.7, left1062 unchanged through 3→0→1→3→0→2. Bottom never exceeds712 at viewport1280×720; overflow scrolls. Physical restoration of old placement is the RED control. All 680 JS tests GREEN. Explicit read-only API fixtures; no live order sent. Geometry/screenshots `/tmp/megaron-march-position-{before,after}/`.
