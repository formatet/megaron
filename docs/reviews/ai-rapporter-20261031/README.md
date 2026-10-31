# AI-spelarnas rapporter 2026-10-31 — bevis

Plan: `.agents/plan-ai-rapporter-20261031.md`. Gren `claude/ai-rapporter-20261031`. Tester: `server/cmd/keryx/ai_reports_20261031_test.go` (keryx), `web/static/js/megaron/ui/citygrid.test.mjs`, `codex.test.mjs`.

| Punkt | Ändring | Test | Mutant (dödad vid) |
|---|---|---|---|
| 1 | `founding status` skriver samma prognos som `settle` (delad `printFoundingForecast`); ny rad `Coastal ground is N step(s) away at (q,r).` (`coastalGroundLine`, serverns `coastal`-flagga, bara kända hexar, 1–2 steg); `settle --help` + Codex `founding.md` pekar på `founding status` | `TestFoundingStatus_PrintsForecastAndNearbyCoast`, `TestCoastalGroundLine`, `TestFoundingSettleHelpPointsAtStatusForecast`, codex.test.mjs:188 | status utan prognosanrop: `ai_reports_20261031_test.go:42/46`; radie 2→1: `:70`; helptext: `:81`; codex-mening: `codex.test.mjs:188` |
| 2 | Varor alfabetiskt efter visad etikett: web `sortGoods` (hex + byggnad), keryx `city` `sortedGoods` | `citygrid.test.mjs:288`, `TestCityCmd_GoodsAlphabeticalPerHex` | sortering bort: `ai_reports_20261031_test.go:27`, `citygrid.test.mjs:288` |
| 3a | `login --help` nämner `POLEIA_PASSWORD` | `TestLoginHelpNamesPasswordEnv` | `:90` |
| 3b | `me` före `join` → "you haven't joined this world yet — run: keryx join" | `TestMeBeforeJoinNamesJoinVerb` | `:107` |
| 3c | `map`-gränslistan markerar hexar som rör känt hav (fog döljer egen terräng — ingen läcka); JSON `borders_known_sea_hexes` | `TestMapFrontierMarksHexesNextToSea` | `:147` |
| 4 | `status`: `Next game day: today 17:30 (in 12 min)` ur världens anker | `TestNextGameDayLine` | `:163/171` |

Sviter: `tools/gotest.sh ./cmd/keryx/` grön, `tools/test-web.sh` grön, `day_guard_test.py` och `test_verb_parity.py` gröna.
