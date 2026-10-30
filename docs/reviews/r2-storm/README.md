# R2 storm, mild — the gods' hand at sea (Claude 2026-10-30)

Canon: vault `megaron_religion_talforslag_20261030.md` §R2, mild column (Timothy 2026-10-30 14:49: "milt överallt");
`megaron_religion_systemforslag_20261028.md` 16:38: the chance shown *before* the action, the god's hand named *in* the outcome.
Gate: proves the religion v2 dice reach the sea (chain: a realm's standing has visible consequences).

## Contract

| Rule | Where |
|---|---|
| Favorable (≥ 60): 15 % a storm met does no harm. Wrathful (< 5): 15 % it takes 1 more hull (3 instead of 2). Otherwise no roll | `religion.StormTilt` (owner of the numbers; combat and kharis both read it — G1 forbids combat→kharis) |
| One roll per storm met, in the same TX as the hull outcome; outcome stored, never the roll | `combat/sea_storm.go sail`, seam `GodDice` |
| Owner without a realm record: no tilt (never read as kharis 0) | same |
| Spared = new event/notice kind `ShipStormSpared` (hull kept, `divine:"spared"`) — `ShipStormDamaged` never means "unharmed" | frozen-semantics rule |
| Wrath = `ShipStormDamaged` with `damage` 3 and `divine:"wrath"` (additive field; damage already varied) | |
| Spared ship still met the storm → may still earn its naming right | `claimStorm` unchanged |
| Chance before sailing: `kharis.DescribeStanding` summary + `storm_spare/wrath_probability` → web Kult drawer, profile, keryx `cult` | `kharis/standing.go` |

Four surfaces: temenos (above) · keryx (`cult` standing, notice lines) · megaron (notice text, icon, map action, Kult summary) ·
Codex (`storms.md` "The gods' hand", `kharis.md` mood table, notice index `sea` kinds).

## Proof

Fresh DB: `TestSeaStorm_GodsHandTiltsTheBlow` (5 cases: spared / favorable unlucky / wrath / wrathful lucky / indifferent),
`TestStandingNamesTheGodsHandAtSea`, `TestPrintSeaStormLine_GodsHand`, web `storm notices: the gods' hand…`.
First run caught a real bug: fixtures without a realm record read as kharis 0 → Wrathful → random wrath in two old storm tests.
Full Go suite + web 881/0 on a fresh DB (mig 233).

Mutants (6/6 killed, each a named red):
- M1 never spare → `sea_storm_test.go:530` (hull 3, want 5)
- M2 no extra hull on wrath → `sea_storm_test.go:530` (hull 3, want 2)
- M3 spared stored as `ShipStormDamaged` → `sea_storm_test.go:538`
- M4 Favorable boundary `>` instead of `>=` → `standing_test.go:37`
- M5 standing summary omits the sea line → `standing_test.go:37`
- M6 missing realm record read as kharis 0 → `sea_storm_test.go:535` (deterministic "no realm record" case; the first M6 run only died by wall-dice luck)

## Not done / next

- BILD: none new (text only). TEXT in playtest: "Poseidon held back the waves: …" and "— the gods' wrath struck harder".
- R2 next slices: battle (Favorable +5 % strength, Wrathful −5 %), then punishment curve (Wrathful 4 %, Suspicious 2 %).
- The storm chance is shown in the standing (Kult/cult), not in the ship's march dialog — a march-time line can come later if playtests miss it.
