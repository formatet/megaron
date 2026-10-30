# R2 battle, mild — the gods' hand in battle (Claude 2026-10-30)

Canon: vault `megaron_religion_talforslag_20261030.md` §R2, mild column ("Favorable +5 % styrka, Wrathful −5 %",
Timothy 2026-10-30 14:49: "milt överallt"); chance shown before the action, the god's hand named in the outcome.
Gate: proves the religion v2 dice reach the battlefield (a realm's standing has visible consequences).

## Contract

| Rule | Where |
|---|---|
| Strength = dice. Favorable (≥ 60) ×1.05, Wrathful (< 5) ×0.95, otherwise none | `religion.BattleDivine` / `BattleFactor` (owner of the numbers) |
| Applied per participant after garrison boost and hull; the fraction is rounded by one roll of the battle's seeded round dice, so a 9-die force keeps its expected tilt. No hand: no roll drawn (old battles' dice streams unchanged) | `combat/battle.go rollSide` |
| The hand is **fixed when a unit enters the battle** (`battle_participants.divine`, mig 234), so a battle stays reproducible from its seed; a later reinforcement carries its owner's mood at its own join | `startBattle`, `joinBattle` |
| Owner without a realm record: no hand (never read as kharis 0) | `battleDivine` |
| Outcome: additive `divine` ("favor"/"wrath") on `BattleWon`/`BattleLost`, only when all of that owner's units carried the same hand. Kinds unchanged | `notifyBattleEnded` |
| Chance before marching: `Standing.BattleStrengthFactor` + summary line "in battle, your men fight about 5% stronger/weaker" | `kharis/standing.go` → web Kult, profile, keryx `cult` |

Four surfaces: temenos (above) · keryx (`cult` standing, battle report line) · megaron (battle notice text, Kult summary) ·
Codex (`battle.md` "The gods' hand", `kharis.md` mood table).

Changed old-rule test: `TestBattleParticipationFixedAcrossCityAndKharis` asserted 45 dice at kharis 1 and 100. Its point
(commitment fixed at 90 %) is kept; dice now 42–43 Wrathful, 47–48 Favorable.

## Proof

Fresh DB: `TestRollSide_GodsHandScalesTheDice` (5 cases incl. both fraction outcomes and "no roll drawn"),
`TestBattle_GodsHandFixedAtJoinAndNamedInOutcome` (snapshot, kharis changed mid-battle, dice still ×1.05, both notices),
`TestBattle_NoRealmRecordCarriesNoHand`, `TestStandingNamesTheGodsHandInBattle`, `TestPrintBattleReportLine_GodsHand`,
web `battle notices: the gods' hand…`. Full Go suite + web 882/0 on a fresh DB (mig 234).

Mutants (9/9 killed, each a named red):
- M1 favor ×1 → `battle_divine_test.go:42`, `fixed_combat_test.go:57`, `standing_test.go:50`
- M2 wrath ×1 → same three
- M3 Favorable boundary `>` → `battle_divine_test.go:77`, `standing_test.go:50`
- M4 hand not stored at join → `battle_divine_test.go:78`
- M5 no realm record read as kharis 0 → `battle_divine_test.go:123` (+ two old battle tests)
- M6 notice omits `divine` → `battle_divine_test.go:101`
- M7 fraction always floored → `battle_divine_test.go:42`
- M8 standing omits the battle line → `standing_test.go:50`
- M9 hand read live each tick instead of frozen → `battle_divine_test.go:100` (survived the first test; the dice assertion was added for it)

## Not done / next

- BILD: none new (text only). TEXT in playtest: "The gods stood with your men." / "The gods' wrath weighed on your men."
- An owner whose units carried different hands (kharis crossed a band between joins) gets no divine line — rare, deliberate.
- R2 next: punishment curve (Wrathful 4 %, Suspicious 2 %).
