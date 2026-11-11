# serverfel — refusals the player can read; disabled buttons that say why (2026-11-11)

Web only. `git diff --stat master -- server` is empty.

## A. `playerError(res, data, fallback, opts)` in `ui/format.js`
Rules in order: logged out (401 with no readable sentence, or "not authenticated"/"unauthorized"/"authentication required") ->
technical (HTTP >= 500, or `isTechnicalError`) -> network (`opts.network`) -> the server's sentence (capitalised, full stop;
a bare code such as `not_owner` is left alone) -> fallback. `formatApiError(data, fb)` is now a wrapper; `insufficient_goods` and
`build_context` text is byte-identical. `refusalText` (citygrid) is a thin caller. `errorDetail`/`setErrorText`/`showInlineResult(...,detail)`
put the raw string in `title` + `data-detail`. Deviations: a 401 whose body is already a sentence (session-expired text, "invalid credentials") keeps it;
the whole prefix "could not " is technical (measured: no "could not ..." is sent with a 4xx).

## Classification (`classification.md`, `server-errors.txt`)
547 distinct literals in `writeError`: 3 logged out, 300 technical, 244 rule refusals. Rule list includes some 5xx strings ("march failed");
those are caught by status wherever the caller passes the Response.

## B. Sites changed (file: before -> after)
- ui/transport_choice.js:30 `esc(d.error||..)` -> `playerError(r,d,..)`
- ui/placename.js:46, ui/stormname.js:53 `textContent = body.error||..` -> `setErrorText(err,res,body,..)`
- ui/recall_all.js:17,31 raw message/`r.error` -> `playerError` network line / `formatApiError`
- render/map.js:3895 `showInlineResult(.., err.error||..)` -> `playerError` + detail; render/map.js:4028 -> `setErrorText`
- ui/marchctx.js:621 `esc(r.data.error)` -> `esc(formatApiError(r.data,''))`
- ui/drawers/send_goods.js:153 `res.data.error||..` -> `playerError`
- ui/drawers/kult.js:225, 64, 79 -> `playerError` + detail
- ui/drawers/city.js:294, 324 -> `playerError` + detail
- ui/drawers/war.js:398 (disband), 441 (abandon), 472/474 (recall), 486 (load forces), 539 (redirect) -> `playerError`/`setErrorText`
- Already on `formatApiError` (now safe): about 45 call sites in diplomacy, recruitment, march_preview, dispatch_window, market_board, account_window, war, marchctx, city.
- Not changed: war.js:528 `out.error` (client-side validation text from `redirectBody`, not a server string); verb_telemetry.js:140 (telemetry, not shown).
Guard: `ui/playererror.test.mjs` "guard" scans all non-test `.js`; zero hits on this branch. Proof: reverted city.js:294 to `d.error || ...` -> guard failed naming that line; restored.
Not covered: `status(...)` closures in war.js carry the friendly text but no detail attribute (no element at hand).

## C. Disabled buttons
With a visible reason now (`.btn-reason`, megaron.css): Fit out (force_card.js), worker grid +1 / -1 (citygrid.js; "All places are staffed", "None of yours to take off"), Recall all (recall_all.js "No forces are marching").
Recruit rows: reason already visible in the cost text (needs a building / missing goods); the in-flight pending state has none (transient).
No reason available client-side: Send goods "Send" (several causes, route/overload lines already visible), settle button (`can_found_here` carries no reason), city drawer nav arrows (<2 cities), diplomacy good select (failed catalogue), Reinforce/others not disabled by state.
Transport route radios and Kult scope radios: reason already printed in the label.

## Pictures (`tools/serverfel_bild.py`, Firefox, real modules + megaron.css, fixture)
E1 rule refusal, E2 technical (friendly line), E3 logged out, E4 Fit out + reason, E5 worker grid at capacity: `E1..E5-desktop.png` (1280); E6-E10 same at 390 (`*-mobile.png`).
Detail attribute (not on screen): E2 `title="could not load goods"`, E3 `data-detail="not authenticated"`.

## Checks
`bash tools/test-web.sh`: 993 pass (982 on master + 11 new). Existing tests updated only where the new contract adds capital + full stop to a rule refusal (citygrid, inline_handlers, placename, disband, founding_inline, recall_all).
Codex: no article quotes a technical string.
