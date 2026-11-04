# Gubbe-lyft: placements lifted on population drop (order 2026-11-02, rewritten 2026-11-04)

Owner: `economy.PrunePlacementsToPopulation` (S1b order untouched), now returns `[]LiftedPlacement`.
Callers: recruit (same tx as the draft), reinforcement (`kharis.debitReinforcement`, own tx), daily tick (cause `decline`).
Other population decrements: `combat/occupation.go:564` and starvation in `kharis/tick.go` applyDecay are caught by the tick prune (decline);
`combat/battle.go` has no debit (left alone). Pure increments (disband, lay-up, cancel_training) need nothing.

## Notification text (for TEXT playtest)
- `A citizen left their work in Gournia: 1 on grain at (30, 45) (men were drafted).`
- `A citizen left their work in Gournia: 1 at the olive press (oil) (the city has fewer people than posts).`
- `A citizen left their work in Gournia: 1 serving at the temple of Potnia (men were sent to reinforce a unit).`
- `3 citizens left their work in Gournia: grain at (30, 45) ×2, olive press (oil) ×1 (men were drafted).`

## Mutants (all killed)
1. recruit skips prune: `recruit_lift_test.go:42`
2. reinforcement skips prune: `placements_lifted_test.go:90`, `:93`
3. tick drops the announce: `placements_lifted_test.go:136`
4. `rn > target` to `rn >= target`: `placement_prune_test.go:36`, `:62`, `:92`, `:102`
