package movement

import "testing"

func TestTimedRoutePrefixFromActualDeparture(t *testing.T) {
	m := Move{Version: 2, OffsetMilli: 733, StartTick: 103, EndTick: 104, Hexes: []Hex{{60, 27}, {59, 27}, {59, 26}, {58, 26}}, Costs: []int64{200, 200, 200}}
	for _, tc := range []struct {
		at   Milli
		want int
	}{{103733, 0}, {104065, 0}, {104066, 1}, {104399, 1}, {104400, 2}, {104732, 2}, {104733, 3}} {
		got, err := PositionAt(m, tc.at)
		if err != nil || got != m.Hexes[tc.want] {
			t.Fatalf("at %d got %v err %v want %v", tc.at, got, err, m.Hexes[tc.want])
		}
	}
}
