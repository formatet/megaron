# Windows get out of each other's way (desktop)

Rule (owner: `web/static/js/megaron/ui/mobile_panels.js`): a window that opens on top of another window closes it,
through the covered window's own close callback. Windows that do not overlap both stay. The newest window wins, also
when it sits beneath the old one (a hex click while a drawer is open closes the stale drawer). Overlap is measured on
the visible box: the dialog inside a modal overlay (not its dimmed backdrop), a drawer at its resting place (slide-in
transform removed). Phone (<= 480 px) behaviour is untouched (still closes the predecessor synchronously).

Tables, produced on the real canvas map in Firefox by `tools/fonster_inventory.py before|after`:
`table-before.md`, `table-after.md` (+ `results-*.json`). Nine windows (hex panel, march menu, drawers city/war/report,
Codex, dispatch, search, account), every ordered pair, 1280x800 and 1920x1080: 132 measured pairs.

| | covered and still open | closed by the overlap |
|---|---|---|
| before | 38 | 18 (drawer vs drawer, hex vs drawer: the only two rules that existed) |
| after | 3 (march-menu popup, see below) | 53 |

No window reappears afterwards: nothing is covered-and-open except the popup case. Pictures of every pair that was
covered-and-open before: `<A>-then-<B>-<width>.png`; the three remaining: same names with `-after`.

## Pairs deliberately left open together (do not overlap)
Codex (left, 440 px) with any drawer (right, 420 px) at 1280 and 1920; the hex panel (right, 220 px) with the Codex;
a centred dialog (search 540, dispatch/account 420) with the hex panel at both widths; march menu opened mid-map with
the hex panel. They are separate on the screen, so closing either would remove something the player can still see.

## For Timothy
The march menu is a small popup at the pointer (right-click, or the March button in the hex panel). When it lands on a
drawer or hex panel it covers it but closes nothing (`openMobilePanel(..., { popup: true })`): both orders are
defensible (closing the city drawer because a right-click menu brushed it is destructive; leaving it is today's
behaviour), so I kept today's. Rows: drawer-city/war/report then march at 1280. Pictures:
`drawer-city-then-march-1280-after.png` (and war, report). A window opened over the menu does close it.
Flip it by deleting `{ popup: true }` in `ui/marchctx.js`.

## Not tried
Hover tooltips, the hex-pick bar and build-pick mode (they already hide panels without registering as windows);
drawers diplomacy/economy/kult (same geometry as the three measured); modal overlays vs hex/march click (the
dimmed backdrop swallows the click, which closes the overlay: not a stale window); a desktop window narrower than
~860 px (Codex + drawer would then overlap and the rule closes the older).

## What the real map showed that tests did not
A drawer's rectangle is wrong while it slides in (translateX still applied when the check runs), and an overlay has a
full-screen backdrop but a small dialog; the fake-DOM tests knew neither. The overlap is therefore taken on the
dialog and with the slide transform removed, and judged one microtask after open, when the caller has shown the window.
