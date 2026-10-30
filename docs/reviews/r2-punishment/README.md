# R2 punishment curve, mild (Claude 2026-10-30)

Canon: vault `megaron_religion_talforslag_20261030.md` §R2, mild column ("Wrathful 4 % · Suspicious 2 % · annars 0",
Timothy 2026-10-30 14:49: "milt överallt"). Was: 3 %/tick below 15. Protection (7 ticks after an actual loss) unchanged.

## Contract

| Rule | Where |
|---|---|
| Risk begins below `WrathThreshold` = `religion.MoodIndifferent` (30): 2 %/tick; below `WrathfulThreshold` = `MoodSuspicious` (5): 4 %; ≥ 30: 0. Temple-independent (R5) | `kharis.PunishmentProbability`, `wrath.go` |
| `WrathStatus` carries both tiers (`wrathful_threshold`, `wrathful_probability`) | Kult rules line, keryx `status` |
| `KharisDeclining` still fires once per downward crossing of 30 with negative net — now the point where risk begins. New notices carry `wrath_probability`, `days_to_wrathful`, `wrathful_threshold`, `wrathful_probability`; `days_to_15` is no longer written. Same kind; old notices keep their fields and old text (web + keryx branch on `days_to_wrathful`) | `transition_notices.go`, `format.js`, `notification_text.go` |

Four surfaces: temenos · keryx (`status`, notice line) · megaron (Kult rules, notice text, standing summary via `PunishmentProbability`) ·
Codex (`kharis.md` mood table + Divine wrath, `temples.md`, `coming-back.md`; `TestKharisPlayerRulesSourceParity` reads the 2 %).

Old-rule tests moved to the new curve: `TestRealmPunishment_BandBoundaries`, `TestWrathThresholdDoesNotDependOnTemple`,
`TestStandingCurrentConsequences`, `TestWrathDeclineNoticeOnceAndNoPositiveNetWarning` (12 → 20 game days, to 5),
`TestOwnerWrathExpiryUsesWorldAnchorAndStaysPrivate`.

## Proof

Full Go suite + web 884/0 on a fresh DB. New text tests: web `kharis decline warning…` (new + old payload),
Kult `R2 punishment curve…`, keryx `TestKharisDecliningLine_PunishmentCurve` (new + old payload).

Mutants (5/5 killed): P1 Wrathful tier missing → `decay_test.go:43` · P2 risk starts at 15 again → `kharis_status_test.go:91` +3 ·
P3 boundary `<=` 30 → `decay_test.go:43` · P4 countdown to 30 instead of 5 → `wrath_test.go:72` · P5 status omits the tier →
`kharis_status_test.go:157`.

## Live consequence (dev world)

Realms already Wrathful go from 3 % to 4 %; realms at 15–30 now carry 2 % with no fresh warning (their crossing of 30
happened under the old text). The R1 transition shield (14 ticks) has already run out in the dev world. Abandoned AI
accounts under 5 will be punished somewhat more often (`DivinePunishment` signal, todo VERIFIERING).

## Not done

- TEXT in playtest: the new decline warning and the Kult line.
- A crossing straight from ≥ 30 to < 5 in one tick reads "falls below 5 in 0 game days" — not reachable at today's decay rates.
