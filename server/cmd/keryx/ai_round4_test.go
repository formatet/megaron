package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

const round4Hex = `{"total_gubbar":10,"pool_size":7,"hexes":[{"hex_ordinal":6,"hex_q":12,"hex_r":-3,"terrain":"forest_olive_grove","building":{"type":"lumbermill","level":1},"goods":[{"good_key":"timber","marginal_yield":1.7,"placed":1,"placed_ordinals":[1]}]}]}`

func TestAIRound4_HexSelectors(t *testing.T) {
	for _, target := range []string{"6", "#6", "12,-3"} {
		for _, kind := range []string{"build", "place"} {
			t.Run(kind+target, func(t *testing.T) {
				posted := false
				round3Client(t, func(w http.ResponseWriter, r *http.Request) {
					if r.Method == "POST" {
						var body map[string]any
						if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
							t.Fatal(err)
						}
						posted = true
						if kind == "build" {
							if body["hex_q"] != float64(12) || body["hex_r"] != float64(-3) {
								t.Errorf("wrong build target: %v", body)
							}
						} else {
							if body["hex_ordinal"] != float64(6) {
								t.Errorf("wrong placement target: %v", body)
							}
						}
						fmt.Fprint(w, `{"gubbe_ordinal":2}`)
						return
					}
					fmt.Fprint(w, round4Hex)
				})
				var cmd *cobra.Command
				if kind == "build" {
					cmd = buildCmd()
					cmd.SetArgs([]string{"--type", "lumbermill", "--hex", target})
				} else {
					cmd = placeCmd()
					cmd.SetArgs([]string{"--hex", target, "timber", "+1"})
				}
				out, err := captureStdout(t, cmd.Execute)
				if err != nil || !posted {
					t.Fatalf("%v posted%v %s", err, posted, out)
				}
			})
		}
	}
	for _, args := range [][]string{{"timber", "#6", "+1"}, {"--hex", "#6", "timber", "-1"}} {
		t.Run(strings.Join(args, "/"), func(t *testing.T) {
			mutated := false
			round3Client(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" {
					mutated = true
					fmt.Fprint(w, `{"gubbe_ordinal":1}`)
					return
				}
				fmt.Fprint(w, round4Hex)
			})
			cmd := placeCmd()
			cmd.SetArgs(args)
			_, err := captureStdout(t, cmd.Execute)
			if err != nil || !mutated {
				t.Fatalf("%v mutation%v", err, mutated)
			}
		})
	}
	t.Run("unseen ordinal is never invented", func(t *testing.T) {
		posted := false
		round3Client(t, func(w http.ResponseWriter, r *http.Request) {
			if r.Method == "POST" {
				posted = true
			}
			fmt.Fprint(w, round4Hex)
		})
		cmd := buildCmd()
		cmd.SetArgs([]string{"--type", "lumbermill", "--hex", "99"})
		_, err := captureStdout(t, cmd.Execute)
		if err == nil || posted || !strings.Contains(err.Error(), "known catchment") {
			t.Fatalf("%v posted%v", err, posted)
		}
	})
}

func TestAIRound4_HexStaffRedirect(t *testing.T) {
	round3Client(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Fatal("staff must not mutate a hex building")
		}
		fmt.Fprint(w, round4Hex)
	})
	cmd := staffCmd()
	cmd.SetArgs([]string{"lumbermill", "+2"})
	_, err := captureStdout(t, cmd.Execute)
	if err == nil || !strings.Contains(err.Error(), "lumbermill is a hex building") || !strings.Contains(err.Error(), "keryx place timber 6 +2") {
		t.Fatal(err)
	}
}

func TestAIRound4_AutomaticFoodAndNoMoveAdvice(t *testing.T) {
	out := runCityCmd(t, `{"total_gubbar":10,"pool_size":7,"hexes":[]}`, `{"settlement":{"food_gubbar_required":5,"food_gubbar_placed":5,"food_self_sufficient":true,"automatic_food":{"nearjord_grain_per_tick":0.5,"remainder_people":42,"remainder_grain_per_tick":0,"remainder_fish_per_tick":0.42}}}`)
	for _, want := range []string{"5 citizens needed", "5 on food", "Automatic food: 0.50 grain + 0.42 fish", "nearby ground 0.50", "42 people outside whole citizens: 0.00 grain + 0.42 fish"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in %s", want, out)
		}
	}
	for _, bad := range []string{"could move", "★", "1 citizens"} {
		if strings.Contains(out, bad) {
			t.Fatal(out)
		}
	}
	line := foodGubbarLine(1, 3, true)
	if !strings.Contains(line, "1 citizen needed") || strings.Contains(line, "could move") {
		t.Fatal(line)
	}
}

func TestAIRound4_NetThresholdAndSingular(t *testing.T) {
	for _, tc := range []struct {
		n    float64
		want string
	}{{.049, "about even"}, {.05, "+0.1"}, {.1, "+0.1"}, {-.05, "-0.1"}, {-.1, "-0.1"}} {
		if got := formatNetPerTick(tc.n); got != tc.want {
			t.Fatalf("%.4f ->%q, want%q", tc.n, got, tc.want)
		}
	}
	if got := armyFoodLine(map[string]any{"food_net_per_tick_after_upkeep": .1, "army_upkeep": map[string]any{"grain": 1.0}}); !strings.Contains(got, "net +0.1 per game day after army") {
		t.Fatal(got)
	}
	for _, n := range []int{0, 1, 2} {
		got := citizenCount(n)
		want := fmt.Sprintf("%d citizens", n)
		if n == 1 {
			want = "1 citizen"
		}
		if got != want {
			t.Fatal(got)
		}
	}
}

func TestAIRound4_QueueExplainsFixedSlots(t *testing.T) {
	round3Client(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"settlement":{"build_queue_max":2,"build_queue":[{},{}]}}`)
	})
	out, err := captureStdout(t, func() error { return printBuildQueue(newClient(cfg), "world", "default") })
	if err != nil || !strings.Contains(out, "2/2 slots used — fixed limit; population and buildings do not add slots") {
		t.Fatalf("%v %s", err, out)
	}
}
