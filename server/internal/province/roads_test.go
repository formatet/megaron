package province

// Vägar V1 (megaron_plan_vagar.md): lagring + kantkostnad. Inga vägar ska
// betyda bit-identiska rutter; faktorerna kommer ur EN tabell (roadFactors).

import (
	"container/heap"
	"context"
	"encoding/json"
	"math"
	"math/rand"
	"reflect"
	"testing"
)

func roadsOf(edges ...[2]MapPosition) RoadSet {
	r := RoadSet{}
	for _, e := range edges {
		r[roadKey(e[0], e[1])] = struct{}{}
	}
	return r
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

// (b) F6: kanten (A,B) och (B,A) är samma rad, i alla sex riktningar.
func TestRoadKeyIsUnordered(t *testing.T) {
	a := MapPosition{Q: 3, R: -2}
	for _, d := range axialDirs {
		b := MapPosition{Q: a.Q + d[0], R: a.R + d[1]}
		if roadKey(a, b) != roadKey(b, a) {
			t.Fatalf("roadKey(%v,%v) != roadKey(%v,%v)", a, b, b, a)
		}
		rs := roadsOf([2]MapPosition{a, b})
		if !rs.Has(a, b) || !rs.Has(b, a) {
			t.Errorf("Has must hold in both directions for %v-%v", a, b)
		}
		k := roadKey(a, b)
		if !(k[0] < k[2] || (k[0] == k[2] && k[1] < k[3])) {
			t.Errorf("key %v is not sorted by (q,r)", k)
		}
	}
	var nilSet RoadSet
	if nilSet.Has(a, a) {
		t.Error("nil RoadSet has no edges")
	}
}

// (d) Faktorerna, hårdkodade mot planens §2 (inte läsa ur tabellen).
func TestEdgeCostFactorsPerCategory(t *testing.T) {
	a, b := MapPosition{Q: 0, R: 0}, MapPosition{Q: 1, R: 0}
	rs := roadsOf([2]MapPosition{a, b})
	cases := []struct {
		category string
		terrain  string
		want     float64
	}{
		{"land", "plains", 1 * 0.75},
		{"land", "hills", 2 * 0.75},
		{CategoryCourier, "plains", 0.5 * 0.70},
		{CategoryCourierLand, "scrub_maquis", 0.75 * 0.70},
		{CategoryCaravan, "plains", 1 * 0.60},
		{CategoryCaravan, "semi_desert", 3 * 0.60},
	}
	for _, c := range cases {
		if got := EdgeCost(a, b, "plains", c.terrain, c.category, rs); !near(got, c.want) {
			t.Errorf("%s -> %s: got %v, want %v", c.category, c.terrain, got, c.want)
		}
		if got := EdgeCost(a, b, "plains", c.terrain, c.category, nil); !near(got, moveHoursFor(c.terrain, c.category)) {
			t.Errorf("%s -> %s without roads must be the plain terrain cost, got %v", c.category, c.terrain, got)
		}
	}
	// Skepp: vägar är land, ingen effekt ens på en rad som (omöjligt) finns.
	for _, terr := range []string{"coastal_sea", "deep_sea"} {
		if got := EdgeCost(a, b, terr, terr, "naval", rs); !near(got, TerrainMoveTicks(terr)) {
			t.Errorf("naval over %s must ignore roads, got %v", terr, got)
		}
	}
	// Väg i motsatt riktning är samma kant.
	if got := EdgeCost(b, a, "plains", "plains", "land", rs); !near(got, 0.75) {
		t.Errorf("reverse direction: got %v, want 0.75", got)
	}
}

// (c) Vad en väg aldrig får påverka: ford/flod/hav/berg som ändpunkt, åt båda håll.
func TestEdgeCostIgnoresNonRoadableEndpoints(t *testing.T) {
	a, b := MapPosition{Q: 0, R: 0}, MapPosition{Q: 1, R: 0}
	rs := roadsOf([2]MapPosition{a, b})
	for _, bad := range []string{"river_ford", "river", "coastal_sea", "deep_sea", "mountain_limestone", "mountain_red"} {
		for _, cat := range []string{"land", CategoryCourier, CategoryCaravan} {
			want := moveHoursFor(bad, cat)
			if got := EdgeCost(a, b, "plains", bad, cat, rs); !near(got, want) {
				t.Errorf("%s: road into %s must not apply, got %v want %v", cat, bad, got, want)
			}
			want = moveHoursFor("plains", cat)
			if got := EdgeCost(a, b, bad, "plains", cat, rs); !near(got, want) {
				t.Errorf("%s: road out of %s must not apply, got %v want %v", cat, bad, got, want)
			}
		}
	}
	// Och alla åtta vägbara terränger tar emot väg.
	for _, ok := range []string{"plains", "river_valley", "river_delta", "forest_olive_grove", "scrub_maquis", "hills", "forest_cedar", "semi_desert"} {
		if got := EdgeCost(a, b, ok, ok, "land", rs); !near(got, moveHoursFor(ok, "land")*0.75) {
			t.Errorf("%s must be roadable, got %v", ok, got)
		}
	}
}

// (a) Väg-omvägen väljs av A*.
//
//	direkt:  (0,0) ─ (1,0) kulle ─ (2,0) kulle ─ (3,0)      = 2+2+1  = 5
//	omväg:   (0,0) ─ (1,-1) macchia ─ (2,-1) macchia ─ (3,-1) macchia ─ (3,0) = 1,5·3+1 = 5,5
//
// Utan väg vinner den direkta; med väg på omvägens fyra kanter kostar den
// 0,75·5,5 = 4,125 och vinner.
func TestRoadDetourIsChosen(t *testing.T) {
	tiles := map[[2]int]string{
		{0, 0}: "plains", {1, 0}: "hills", {2, 0}: "hills", {3, 0}: "plains",
		{1, -1}: "scrub_maquis", {2, -1}: "scrub_maquis", {3, -1}: "scrub_maquis",
	}
	o, tg := MapPosition{Q: 0, R: 0}, MapPosition{Q: 3, R: 0}
	path, cost, ok := findPathRoads(tiles, o, tg, "land", nil)
	if !ok || len(path) != 4 || !near(cost, 5) {
		t.Fatalf("without roads: path=%v cost=%v ok=%v, want the 4-hex direct route at 5", path, cost, ok)
	}
	rs := roadsOf(
		[2]MapPosition{{Q: 0, R: 0}, {Q: 1, R: -1}},
		[2]MapPosition{{Q: 1, R: -1}, {Q: 2, R: -1}},
		[2]MapPosition{{Q: 2, R: -1}, {Q: 3, R: -1}},
		[2]MapPosition{{Q: 3, R: -1}, {Q: 3, R: 0}},
	)
	path, cost, ok = findPathRoads(tiles, o, tg, "land", rs)
	if !ok || len(path) != 5 || !near(cost, 4.125) {
		t.Fatalf("with roads: path=%v cost=%v ok=%v, want the 5-hex road detour at 4.125", path, cost, ok)
	}
	// StepHours prissätter samma väg med samma kostnadsfunktion.
	var sum float64
	for _, s := range TileGraph(tiles).StepHours(path, "land", rs) {
		sum += s
	}
	if !near(sum, cost) {
		t.Errorf("StepHours sum %v != A* cost %v", sum, cost)
	}
	// En lika dyr väg åt bud: ×0,70 på halva terrängkostnaden.
	_, ccost, cok := findPathRoads(tiles, o, tg, CategoryCourier, rs)
	if !cok || !near(ccost, 5.5/2*0.70) {
		t.Errorf("courier on road detour: cost %v ok=%v, want %v", ccost, cok, 5.5/2*0.70)
	}
}

// refDijkstra är testets egen orakel: samma EdgeCost men ren Dijkstra, ingen heuristik.
func refDijkstra(tiles map[[2]int]string, o, t MapPosition, cat string, roads RoadSet) (float64, bool) {
	if !isPassable(tiles[[2]int{o.Q, o.R}], cat) || !isPassable(tiles[[2]int{t.Q, t.R}], cat) {
		return 0, false
	}
	dist := map[MapPosition]float64{o: 0}
	pq := &aStarQueue{}
	heap.Push(pq, &aStarItem{pos: o, g: 0, f: 0})
	for pq.Len() > 0 {
		cur := heap.Pop(pq).(*aStarItem)
		if cur.g > dist[cur.pos]+1e-12 {
			continue
		}
		if cur.pos == t {
			return cur.g, true
		}
		for _, d := range axialDirs {
			n := MapPosition{Q: cur.pos.Q + d[0], R: cur.pos.R + d[1]}
			nt, ex := tiles[[2]int{n.Q, n.R}]
			if !ex || !isPassable(nt, cat) {
				continue
			}
			ng := cur.g + EdgeCost(cur.pos, n, tiles[[2]int{cur.pos.Q, cur.pos.R}], nt, cat, roads)
			if old, seen := dist[n]; !seen || ng < old-1e-12 {
				dist[n] = ng
				heap.Push(pq, &aStarItem{pos: n, g: ng, f: ng})
			}
		}
	}
	return 0, false
}

// (f) F1: heuristiken måste skalas med minsta vägfaktor. Slumpgrafer med många
// vägar; A* ska ge exakt Dijkstras kostnad. Muteras faktorn ur
// minPassableCost blir A* tyst icke-optimal och det här testet rött.
func TestAStarStaysOptimalWithRoads(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	terrains := []string{"plains", "plains", "plains", "hills", "scrub_maquis", "forest_cedar", "semi_desert", "river_valley"}
	const W = 9
	for round := 0; round < 40; round++ {
		tiles := map[[2]int]string{}
		for q := 0; q < W; q++ {
			for r := 0; r < W; r++ {
				tiles[[2]int{q, r}] = terrains[rng.Intn(len(terrains))]
			}
		}
		roads := RoadSet{}
		for q := 0; q < W; q++ {
			for r := 0; r < W; r++ {
				for _, d := range axialDirs {
					if rng.Intn(100) < 55 {
						roads[roadKey(MapPosition{Q: q, R: r}, MapPosition{Q: q + d[0], R: r + d[1]})] = struct{}{}
					}
				}
			}
		}
		for _, cat := range []string{"land", CategoryCourier, CategoryCourierLand, CategoryCaravan, CategoryWarChariot} {
			for i := 0; i < 12; i++ {
				o := MapPosition{Q: rng.Intn(W), R: rng.Intn(W)}
				tg := MapPosition{Q: rng.Intn(W), R: rng.Intn(W)}
				want, wok := refDijkstra(tiles, o, tg, cat, roads)
				path, got, ok := findPathRoads(tiles, o, tg, cat, roads)
				if ok != wok {
					t.Fatalf("round %d %s %v->%v: ok=%v want %v", round, cat, o, tg, ok, wok)
				}
				if ok && !near(got, want) {
					t.Fatalf("round %d %s %v->%v: A* cost %v, optimal %v (path %v) — heuristic not admissible?", round, cat, o, tg, got, want, path)
				}
			}
		}
	}
}

// (Bevis) Inga vägar ⇒ identiska rutter: nil och tom mängd ger samma väg och
// kostnad, och kostnaden är exakt summan av den gamla terrängprissättningen.
func TestNoRoadsIsIdenticalToTerrainPricing(t *testing.T) {
	rng := rand.New(rand.NewSource(11))
	terrains := []string{"plains", "hills", "scrub_maquis", "forest_cedar", "semi_desert", "river_ford", "river", "coastal_sea", "mountain_red", "forest_olive_grove"}
	const W = 8
	tiles := map[[2]int]string{}
	for q := 0; q < W; q++ {
		for r := 0; r < W; r++ {
			tiles[[2]int{q, r}] = terrains[rng.Intn(len(terrains))]
		}
	}
	for _, cat := range []string{"land", "naval", CategoryCourier, CategoryCourierLand, CategoryCaravan, CategoryWarChariot} {
		for i := 0; i < 60; i++ {
			o := MapPosition{Q: rng.Intn(W), R: rng.Intn(W)}
			tg := MapPosition{Q: rng.Intn(W), R: rng.Intn(W)}
			p1, c1, ok1 := findPath(tiles, o, tg, cat)
			p2, c2, ok2 := findPathRoads(tiles, o, tg, cat, RoadSet{})
			if ok1 != ok2 || c1 != c2 || !reflect.DeepEqual(p1, p2) {
				t.Fatalf("%s %v->%v: nil vs empty RoadSet differ: %v %v %v / %v %v %v", cat, o, tg, p1, c1, ok1, p2, c2, ok2)
			}
			if ok1 {
				var sum float64
				for _, h := range p1[1:] {
					sum += moveHoursFor(tiles[[2]int{h.Q, h.R}], cat)
				}
				if c1 != sum {
					t.Fatalf("%s %v->%v: cost %v != legacy terrain sum %v", cat, o, tg, c1, sum)
				}
			}
		}
	}
}

// En karavan prissätts i sin egen vägklass men journeyns Category förblir "land".
func TestPlanTradeJourneyUsesCaravanRoadFactor(t *testing.T) {
	g := TileGraph{{0, 0}: "plains", {1, 0}: "plains", {2, 0}: "plains"}
	rs := roadsOf(
		[2]MapPosition{{Q: 0, R: 0}, {Q: 1, R: 0}},
		[2]MapPosition{{Q: 1, R: 0}, {Q: 2, R: 0}},
	)
	j, err := g.PlanTradeJourney(MapPosition{}, MapPosition{Q: 2}, "land", rs)
	if err != nil {
		t.Fatal(err)
	}
	if j.Category != "land" {
		t.Errorf("journey category = %q, want land", j.Category)
	}
	for i, c := range j.StepCosts {
		if c != 600 {
			t.Errorf("step %d = %d milli, want 600 (plains 1 × caravan 0.60)", i, c)
		}
	}
	plain, _ := g.PlanTradeJourney(MapPosition{}, MapPosition{Q: 2}, "land", nil)
	if plain.StepCosts[0] != 1000 {
		t.Errorf("roadless step = %d, want 1000", plain.StepCosts[0])
	}
}

// --- DB: raden är sanningen -------------------------------------------------

func TestRoadEdgesTableAndLoad(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	world := testWorld(t, pool)
	for _, tl := range []struct {
		q, r int
		t    string
	}{{0, 0, "plains"}, {1, 0, "plains"}, {2, 0, "plains"}, {1, 1, "plains"}} {
		if _, err := pool.Exec(ctx, `INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1,$2,$3,$4)`, world, tl.q, tl.r, tl.t); err != nil {
			t.Fatal(err)
		}
	}
	ins := func(q1, r1, q2, r2 int) error {
		_, err := pool.Exec(ctx, `INSERT INTO road_edges (world_id, q1,r1,q2,r2, built_tick) VALUES ($1,$2,$3,$4,$5,0)`, world, q1, r1, q2, r2)
		return err
	}
	// CHECK: oordnad rad och icke-grannar vägras av schemat.
	if ins(1, 0, 0, 0) == nil {
		t.Error("non-canonical order (1,0)-(0,0) must be rejected by the CHECK")
	}
	if ins(0, 0, 2, 0) == nil {
		t.Error("non-neighbours must be rejected by the CHECK")
	}
	if err := ins(0, 0, 1, 0); err != nil {
		t.Fatalf("canonical neighbour edge: %v", err)
	}
	if ins(0, 0, 1, 0) == nil {
		t.Error("duplicate edge must hit the primary key")
	}
	if err := ins(1, 0, 2, 0); err != nil {
		t.Fatal(err)
	}

	roads, err := LoadRoadSet(ctx, pool, world)
	if err != nil {
		t.Fatal(err)
	}
	if len(roads) != 2 || !roads.Has(MapPosition{Q: 1, R: 0}, MapPosition{Q: 0, R: 0}) || !roads.Has(MapPosition{Q: 1, R: 0}, MapPosition{Q: 2, R: 0}) {
		t.Fatalf("loaded %v", roads)
	}

	// Alla prissättare läser raden: A*, StepHoursDB och PlanTradeJourney (DB).
	o, tg := MapPosition{Q: 0, R: 0}, MapPosition{Q: 2, R: 0}
	for _, c := range []struct {
		cat  string
		want float64
	}{{"land", 1.5}, {CategoryCourier, 0.7}, {CategoryCourierLand, 0.7}} {
		path, cost, ok, err := FindPath(ctx, pool, world, o, tg, c.cat)
		if err != nil || !ok || !near(cost, c.want) {
			t.Fatalf("%s: cost=%v ok=%v err=%v, want %v", c.cat, cost, ok, err, c.want)
		}
		steps, err := StepHoursDB(ctx, pool, world, path, c.cat)
		if err != nil || len(steps) != 2 || !near(steps[0]+steps[1], c.want) {
			t.Fatalf("%s: StepHoursDB %v err=%v", c.cat, steps, err)
		}
	}
	j, err := PlanTradeJourney(ctx, pool, world, o, tg, "land")
	if err != nil || j.StepCosts[0] != 600 || j.StepCosts[1] != 600 {
		t.Fatalf("caravan journey from DB rows: %+v err=%v", j, err)
	}

	// (e) F3: en redan planerad resa är fryst; en kant som tillkommer påverkar
	// bara nästa plan. (1,1) är grannen (1,0)-(1,1); lägg en ny väg där.
	before, _ := json.Marshal(j)
	if err := ins(1, 0, 1, 1); err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(j)
	if string(before) != string(after) {
		t.Fatal("a planned journey must not change when an edge is added")
	}
	j2, err := PlanTradeJourney(ctx, pool, world, MapPosition{Q: 1, R: 0}, MapPosition{Q: 1, R: 1}, "land")
	if err != nil || j2.StepCosts[0] != 600 {
		t.Fatalf("next journey must see the new edge: %+v err=%v", j2, err)
	}
}
