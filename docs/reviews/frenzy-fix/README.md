# Battle Frenzy fix

Bug: `EffectBattleFrenzy` wrote `settlements.battle_frenzy_until` (wall clock) that no battle read.

## Change
- `handlers/settlement.go`: frenzy cast shares the `EffectGarrisonDefense` branch (`garrison_defense_until_tick` + `garrison_defense_owner_id`, `DurationTicks(GarrisonDefenseTicks, giftFactor)`); `applyBattleFrenzy` and the `alreadyFrenzied` pre-read removed; the "already active" guard now uses `PrayerActiveRemaining` for frenzy too (same 409 text as before); `battle_frenzy_until` removed from the settlement GET response (no reader in web/keryx).
- `religion/cooldown.go`: `PrayerActiveRemaining` treats `battle_frenzy` like `garrison_defense` (owner-stamped).
- `handlers/cult_prayers.go`: catalogue reads active days and base duration for frenzy.
- `handlers/god.go`: `frenzied` now = garrison defence active for its owner.
- `religion/prayers.go`: six frenzy descriptions say what they do.
- Codex `rites.md` (+ regenerated `articles.json`): paragraph on the war prayers.
- Column `battle_frenzy_until` left in place, unused (no migration). RiteCast `effect` payload for frenzy is now `{until_tick, strength_factor, duration_ticks}` instead of `{expires_at}`; no reader of the old key exists (checked web, keryx).

## Tests
- `TestBattleFrenzyWritesOwnerStampedGarrisonDefense` (handlers/religion_alpha_test.go): cast writes until/owner, stacking refused (409), stranger and ownerless city see 0 remaining.
- Battle effect itself: existing `TestAlphaWallsRealDefenderStrengthAndOwnerExpiry` (combat) covers the shared fields; no separate frenzy battle test since the mechanism is the same columns.

## Mutants
- (a) frenzy branch omits owner_id: killed, `religion_alpha_test.go:363`.
- (b) frenzy removed from the branch (does nothing): killed, `religion_alpha_test.go:360`.

## Full suite
Go: `tools/gotest.sh` whole suite, no FAIL lines. JS: all 905 node tests pass (incl. codex.test.mjs).
