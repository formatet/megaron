# Småuppföljningar (todo NU 5) — proof package, 2026-11-07

Branch `smauppfoljningar` from master `c35d3926`. Order: `.agents/order-smauppfoljningar-20261107.md`. Not pushed, not merged.

| # | Problem -> fix | Tests | Mutants (red at) |
|---|---|---|---|
| 1 | Cargo lists in notices followed payload order -> sorted by good_key (web `fmtGoods` + storm cargo; keryx caravan, storm, `notification_text.go` goods) | `format.test.mjs` "cargo lists are written alphabetically"; keryx `TestCargoListsAlphabetical` | web sort removed -> `format.test.mjs:433,840`; keryx caravan sort -> `cmd_notifications_caravan_test.go:34`; storm sort -> `:40`; transfer sort -> `:46` |
| 2 | Dead `fmtFlows` + its test removed (grep: no other caller) | build/vet | n/a |
| 3 | Codex prose repeated 0.58 / 0.33 / 40-50-75-100 -> points at the generated tables (no inline marker system exists in `codex_facts.py`, only block markers); `articles.json` regenerated | `codex_facts.py --check`, `codex.test.mjs` 29/29 | table value changed -> `kharis_status_test.go` red |
| 4 | keryx `locationStr` wrote garrison city as id and `[:8]` panicked on short ids -> city name from `homes` ("in Knossos"), id fallback via existing `shortID` (panic-free) | `TestLocationStr_Garrisoned_NamesCity` | name lookup disabled -> `cmd_unit_forwardpost_test.go:157`; unsafe slice -> panic in same test |
| 5 | New-Wanax message: `GET founding/status` gained `announces_entry` (= !is_refugee; the only server change). Web hides the field when false, `maxlength` + "N/280" counter. keryx refuses `--message` for a returning Wanax and >280 runes (limit named) before the irreversible act; `founding status` offers `--message` only when announced. Codex `a-new-wanax` updated | server `TestWanaxEntered_StatusSaysWhetherSettlingAnnounces`, `_TooLongNamesTheLimit`; web `founding_inline.test.mjs` "message field"; keryx `TestSettleMessage`, `TestFoundingStatusOffersMessageOnlyWhenAnnounced` | status always true -> `wanax_entered_test.go:254`; web hide removed -> `founding_inline.test.mjs:73`; keryx refugee check off -> `cmd_founding_test.go:78`; limit off -> `:75` |

## Deviation to know about
`TestKharisPlayerRulesSourceParity` (server/api/handlers/kharis_status_test.go) pinned the exact prose strings point 3 removes
("no temple **40**", "**0.58 x ...**"). It now pins the same source values against the generated table rows
instead (own commit). Same guard on the same numbers, different surface.

## Notes
- keryx refuses (does not silently drop) `--message` from a returning Wanax; an older server without the field keeps old behaviour (field shown, no refusal).
- `cmd_hex.go` still calls `locationStr(c, u, nil)`, so a garrisoned unit there shows a short id (hex view has no settlement map loaded). Left alone.
- Pre-existing gofmt drift in `cmd_ticklog.go` / `cmd_codex.go` untouched.
- Verb list: no verb changed (a flag's validation only); `megaron_verblista.md` not edited.
