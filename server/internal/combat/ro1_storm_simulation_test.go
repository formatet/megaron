package combat

// Explicit analysis, skipped by the normal suite. Both arms execute their own
// production weather, route clock and terrain planner. This is a synthetic sea,
// not a live-world acceptance claim. Run tools/ro1_storm_simulation.sh.
import (
	"encoding/json"
	"flag"
	"math/rand"
	"os"
	"testing"

	"formatet/megaron/server/internal/hexgrid"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/unit"
)

var ro1StormOutput = flag.String("ro1-storm-output", "", "write paired RÖ1 storm analysis")

type ro1StormResult struct {
	Terrain   string  `json:"terrain"`
	Mode      string  `json:"mode"`
	Steps     int     `json:"steps"`
	Duration  int     `json:"duration_ticks"`
	Journeys  int     `json:"journeys"`
	Foundered int     `json:"foundered"`
	Percent   float64 `json:"percent"`
}

func TestRO1StormSimulation(t *testing.T) {
	if *ro1StormOutput == "" {
		t.Skip("explicit simulation flag required")
	}
	const worlds = 500
	const samples = 20
	const maxTick = 70
	sea := []hexgrid.Coord{}
	graph := province.TileGraph{}
	for q := 0; q < 80; q++ {
		for r := 0; r < 40; r++ {
			c := hexgrid.Coord{Q: q, R: r}
			sea = append(sea, c)
			graph[[2]int{q, r}] = "deep_sea"
		}
	}
	isSea := func(c hexgrid.Coord) bool { return c.Q >= 0 && c.Q < 80 && c.R >= 0 && c.R < 40 }
	results := []ro1StormResult{}
	for _, terrain := range []string{"coastal_sea", "deep_sea"} {
		for _, mode := range []string{"galley", "war_galley", "cargo"} {
			for _, steps := range []int{10, 20, 40} {
				results = append(results, ro1StormResult{Terrain: terrain, Mode: mode, Steps: steps})
			}
		}
	}
	for seed := 0; seed < worlds; seed++ {
		dice := rand.New(rand.NewSource(int64(2026101200 + seed)))
		used := map[hexgrid.Coord]bool{}
		shapes := []StormShape{}
		headings := []int{}
		for i := 0; i < len(sea)/SeaHexesPerStorm; i++ {
			s, ok := seedShape(sea, used, isSea, dice)
			if !ok {
				t.Fatal("synthetic storm seeding failed")
			}
			shapes = append(shapes, s)
			headings = append(headings, dice.Intn(6))
			for _, c := range s {
				used[c] = true
			}
		}
		occupied := make([]map[[2]int]bool, maxTick+1)
		for tick := 0; tick <= maxTick; tick++ {
			occupied[tick] = map[[2]int]bool{}
			for i, s := range shapes {
				for _, c := range s {
					occupied[tick][[2]int{c.Q, c.R}] = true
				}
				if tick < maxTick {
					shapes[i], headings[i], _ = StepStorm(s, headings[i], isSea, dice)
				}
			}
		}
		positions := rand.New(rand.NewSource(int64(100000 + seed)))
		for sample := 0; sample < samples; sample++ {
			q, r, start := positions.Intn(40), positions.Intn(40), positions.Intn(21)
			for i := range results {
				result := &results[i]
				g := province.TileGraph{}
				path := []province.MapPosition{}
				for step := 0; step <= result.Steps; step++ {
					p := province.MapPosition{Q: q + step, R: r}
					path = append(path, p)
					g[[2]int{p.Q, p.R}] = result.Terrain
				}
				costs := g.StepHours(path, "naval")
				var route StoredRoute
				if result.Mode == "cargo" {
					j, err := g.PlanTradeJourney(path[0], path[len(path)-1], "naval")
					if err != nil {
						t.Fatal(err)
					}
					raw, _ := json.Marshal(j)
					var ok bool
					route, ok = journeyRoute(raw, start, start+j.TravelTicks)
					if !ok {
						t.Fatal("cargo route invalid")
					}
				} else {
					kind := unit.TypeGalley
					if result.Mode == "war_galley" {
						kind = unit.TypeWarGalley
					}
					total := 0.0
					for _, cost := range costs {
						total += cost
					}
					duration := legTravelTicks(kind, unit.CrewFor(kind), false, total)
					var ok bool
					route, ok = BuildRoute(path, costs, start, start+duration)
					if !ok {
						t.Fatal("march route invalid")
					}
				}
				if route.EndTick > maxTick {
					t.Fatal("weather horizon too short")
				}
				enter, err := RouteEnterMilli(route)
				if err != nil {
					t.Fatal(err)
				}
				// Mirror sea_storm.go holds/inStorm and tick-loop exactly: occupancy at
				// integer ticks, including arrival, and no second blow on consecutive
				// occupied ticks. Crossing an intervening hex within a tick isn't sampled.
				hit := func(tick int) bool {
					if tick > route.EndTick || int64(tick)*1000 < enter[0] {
						return false
					}
					idx := 0
					for j, when := range enter {
						if when <= int64(tick)*1000 {
							idx = j
						}
					}
					return occupied[tick][route.Hexes[idx]]
				}
				hull := HullMax
				for tick := start + 1; tick <= route.EndTick; tick++ {
					if hit(tick) && !hit(tick-1) {
						hull -= StormDamage
					}
					if hull <= 0 {
						break
					}
				}
				result.Duration = route.EndTick - start
				result.Journeys++
				if hull <= 0 {
					result.Foundered++
				}
			}
		}
	}
	for i := range results {
		results[i].Percent = 100 * float64(results[i].Foundered) / float64(results[i].Journeys)
	}
	report := struct {
		Worlds   int              `json:"independent_weather_seeds"`
		Samples  int              `json:"paired_lanes_per_world"`
		SeaHexes int              `json:"sea_hexes"`
		Storms   int              `json:"storms_per_world"`
		Hull     int              `json:"initial_hull"`
		Damage   int              `json:"storm_damage"`
		Results  []ro1StormResult `json:"results"`
	}{worlds, samples, len(sea), len(sea) / SeaHexesPerStorm, HullMax, StormDamage, results}
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(*ro1StormOutput, append(raw, '\n'), 0644); err != nil {
		t.Fatal(err)
	}
}
