package province

// Strömmen i floden (Timothy 2026-10-28): skepp medströms ×0,5, motströms ×2,
// tvärs/okänt oförändrat; bara naval. Order: .agents/order-flodstrom-20261028.md.

import (
	"context"
	"testing"

	"formatet/megaron/server/internal/world"
)

// A river running west into the sea: sea (0,0) ← (1,0) ← (2,0) ← (3,0) source.
// Edge 1 is (-1,0) (downstream), edge 0 is (1,0) (upstream).
func westwardRiver() (TileGraph, RoadSet) {
	g := TileGraph{{0, 0}: "coastal_sea", {1, 0}: "river", {2, 0}: "river_ford", {3, 0}: "river", {4, 0}: "river"}
	r := NewRoadSet()
	r.addCurrent(MapPosition{Q: 1}, 1<<0, -1)
	r.addCurrent(MapPosition{Q: 1}, 1<<1, +1)
	r.addCurrent(MapPosition{Q: 2}, 1<<0, -1)
	r.addCurrent(MapPosition{Q: 2}, 1<<1, +1)
	r.addCurrent(MapPosition{Q: 3}, 1<<1, +1) // (3,0) is the source: no upstream edge; (4,0) has unknown flow
	return g, r
}

func TestRiverEdgeOrderMatchesWorld(t *testing.T) {
	for i, d := range axialDirs {
		dq, dr, ok := world.RiverEdgeDelta(i)
		if !ok || dq != d[0] || dr != d[1] {
			t.Fatalf("edge %d: province %v, world (%d,%d)", i, d, dq, dr)
		}
	}
}

func TestShipRidesTheCurrent(t *testing.T) {
	g, roads := westwardRiver()
	step := func(fq, tq int, cat string) float64 {
		from, to := MapPosition{Q: fq}, MapPosition{Q: tq}
		return EdgeCost(from, to, g[[2]int{fq, 0}], g[[2]int{tq, 0}], cat, roads)
	}
	for _, c := range []struct {
		name   string
		fq, tq int
		cat    string
		want   float64
	}{
		{"downstream river", 3, 2, "naval", 0.5},       // into the ford: 1 × 0.5
		{"downstream into river", 2, 1, "naval", 0.25}, // 0.5 × 0.5
		{"upstream river", 1, 2, "naval", 2},           // ford 1 × 2
		{"upstream from ford", 2, 3, "naval", 1},       // 0.5 × 2
		{"in through the mouth", 0, 1, "naval", 1},     // only (1,0)'s mask says so
		{"out to sea", 1, 0, "naval", 1.0 / 3},         // sea is priced as sea
		{"unknown flow", 3, 4, "naval", 0.5},           // neither (3,0) nor (4,0) records a direction between them
		{"runner unaffected", 2, 1, CategoryCourier, 0.5},
	} {
		if got := step(c.fq, c.tq, c.cat); !near(got, c.want) {
			t.Errorf("%s: %v→%v %s = %v, want %v", c.name, c.fq, c.tq, c.cat, got, c.want)
		}
	}
	if !near(EdgeCost(MapPosition{Q: 2}, MapPosition{Q: 1}, "river_ford", "river", "naval", nil), 0.5) {
		t.Error("a nil set must price plain terrain")
	}
}

func TestRiverRoundTripDiffers(t *testing.T) {
	g, roads := westwardRiver()
	down, downCost, ok1 := g.FindPath(MapPosition{Q: 3}, MapPosition{Q: 0}, "naval", roads)
	up, upCost, ok2 := g.FindPath(MapPosition{Q: 0}, MapPosition{Q: 3}, "naval", roads)
	if !ok1 || !ok2 {
		t.Fatal("river not navigable")
	}
	if !(downCost < upCost) {
		t.Fatalf("downstream %v should beat upstream %v", downCost, upCost)
	}
	for _, p := range []struct {
		path []MapPosition
		cost float64
	}{{down, downCost}, {up, upCost}} {
		sum := 0.0
		for _, s := range g.StepHours(p.path, "naval", roads) {
			sum += s
		}
		if !near(sum, p.cost) {
			t.Errorf("StepHours %v ≠ A* %v on %v", sum, p.cost, p.path)
		}
	}
}

func TestLoadWorldGraphReadsRiverCurrent(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	w := testWorld(t, pool)
	for _, tl := range []struct {
		q, r            int
		t               string
		edges, up, down any
		known           any
	}{
		{0, 0, "coastal_sea", nil, nil, nil, nil},
		{1, 0, "river", 3, 1 << 0, 1 << 1, true},
		{2, 0, "river", 3, 0, 1 << 1, true},
		{3, 0, "river", 3, 0, 0, false}, // flow not known: no current either way
	} {
		if _, err := pool.Exec(ctx, `INSERT INTO map_tiles (world_id, q, r, terrain, river_edges, river_upstream_edges, river_downstream_edges, river_flow_known)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`, w, tl.q, tl.r, tl.t, tl.edges, tl.up, tl.down, tl.known); err != nil {
			t.Fatal(err)
		}
	}
	_, roads, err := LoadWorldGraph(ctx, pool, w)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		fq, tq int
		want   int8
	}{{2, 1, +1}, {1, 2, -1}, {0, 1, -1}, {1, 0, +1}, {3, 2, 0}, {2, 3, 0}} {
		if got := roads.Current(MapPosition{Q: c.fq}, MapPosition{Q: c.tq}); got != c.want {
			t.Errorf("current %d→%d = %d, want %d", c.fq, c.tq, got, c.want)
		}
	}
}
