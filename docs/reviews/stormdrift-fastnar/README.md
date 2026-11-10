# stormdrift-fastnar proof package

- Baseline (live-terrain replay, 500 seeds x 60 ticks): worst stationary run 23; histogram by 5: 0:224 5:193 10:58 15:17 20:8.
- Order's dice-only +-1 turn: worst 16 (0:426 5:67 10:6 15:1) - not enough for the <= 8 criterion.
- Fixed (turn toward the side with a forward step, dice on a tie): worst 6; 0:488 5:12.
- Open-sea guard (steady dice, 3 shapes x 6 headings, 30 ticks): >= 15 hexes before and after.
- Mutants: condition removed -> coast test red (worst 23); `best > 1e-9` -> coast test red (28) + open-sea test red (line heading 0: 7.5); turn kept but step skipped -> TestStepStorm_OneAndTwoHexStormsStayConnectedAndOnSea red (storm_lifecycle_test.go:119).
