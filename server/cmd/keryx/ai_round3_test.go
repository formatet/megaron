package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

func round3Client(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	old, oldJSON := cfg, jsonMode
	t.Cleanup(func() { cfg, jsonMode = old, oldJSON })
	cfg = &Config{Server: srv.URL, Token: "test", WorldID: "world", ProvinceID: "default"}
	jsonMode = false
	t.Setenv("POLEIA_CONFIG", filepath.Join(t.TempDir(), "config.json"))
}

func TestAIRound3_StatusArmyAndSingleFoodAdvice(t *testing.T) {
	round3Client(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/provinces/default") {
			fmt.Fprint(w, `{"settlement":{"name":"Scheria","population":1000,"labor_pool":1000,"grain_prod_rate":0.5,"grain_consum_rate":5,"food_gubbar_required":2,"food_gubbar_placed":5,"food_self_sufficient":true,"army_upkeep":{"grain":2,"silver":0},"net_grain_per_tick_after_upkeep":-6.5,"net_silver_per_tick_after_upkeep":0,"food_net_per_tick_after_upkeep":-1.5,"sitos":{"food_net_per_tick":0.5},"resources":{"grain":{"amount":10000,"rate":0.5},"fish":{"amount":10000,"rate":5}}}}`)
		} else if strings.HasSuffix(r.URL.Path, "/placements") {
			fmt.Fprint(w, `{"placements":[{"good_key":"grain"},{"good_key":"grain"},{"good_key":"fish"},{"good_key":"fish"},{"good_key":"fish"}]}`)
		} else {
			fmt.Fprint(w, `[]`)
		}
	})
	out, err := captureStdout(t, statusCmd().Execute)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"net +0.5 per day (before army)", "eats 2.0 grain per day → food net -1.5 per day after army", "2 citizens needed for people on best food slots", "1 citizen = 100 people", "Labor: 10 citizens"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q: %s", want, out)
		}
	}
	for _, bad := range []string{"Net      net", "-6.5 grain", "with no receiver"} {
		if strings.Contains(out, bad) {
			t.Fatal(out)
		}
	}
}

func TestAIRound3_ProvinceForms(t *testing.T) {
	for _, makeCmd := range []func() *cobra.Command{cityCmd, placementsAliasCmd, statusCmd, buildCmd, placeCmd} {
		for _, flag := range []bool{false, true} {
			cmd := makeCmd()
			t.Run(fmt.Sprintf("%s/flag%v", cmd.Name(), flag), func(t *testing.T) {
				hit := false
				round3Client(t, func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/api/v1/worlds/world/provinces" {
						fmt.Fprint(w, `[{"id":"selected","name":"Knossos"}]`)
						return
					}
					if strings.Contains(r.URL.Path, "/provinces/selected") {
						hit = true
					}
					switch {
					case strings.HasSuffix(r.URL.Path, "/placement-options"):
						fmt.Fprint(w, `{"total_gubbar":10,"pool_size":5,"hexes":[]}`)
					case strings.HasSuffix(r.URL.Path, "/placements"):
						fmt.Fprint(w, `{"gubbe_ordinal":1}`)
					default:
						fmt.Fprint(w, `{"settlement":{"name":"Knossos"}}`)
					}
				})
				args := []string{"Knossos"}
				if flag {
					args = []string{"--province", "Knossos"}
				}
				if cmd.Name() == "place" {
					args = append(args, "grain", "13")
				}
				if cmd.Name() == "build" {
					args = append(args, "--queue")
				}
				cmd.SetArgs(args)
				_, err := captureStdout(t, cmd.Execute)
				if err != nil {
					t.Fatal(err)
				}
				if !hit {
					t.Fatalf("selector never reached selected province: %v", args)
				}
			})
		}
	}
}

func TestAIRound3_EmptyUnplacePointsToOccupiedGood(t *testing.T) {
	deletes := 0
	round3Client(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" {
			deletes++
		}
		fmt.Fprint(w, `{"total_gubbar":10,"pool_size":5,"hexes":[{"hex_ordinal":1,"goods":[{"good_key":"fish","placed":0}]},{"hex_ordinal":13,"goods":[{"good_key":"fish","placed":2,"placed_ordinals":[1,2]}]}]}`)
	})
	out, err := captureStdout(t, func() error { return unplaceMany(newClient(cfg), "/unused", "world", "default", 1, "fish", 1) })
	if err != nil || deletes != 0 || !strings.Contains(out, "#13 fish (2 citizens)") {
		t.Fatalf("%v deletes%d %s", err, deletes, out)
	}
}

func TestAIRound3_GoodsCitizenScale(t *testing.T) {
	round3Client(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[{"key":"fish","labor_pool":1000,"idle_citizens":300,"employed_citizens":200,"producible":true,"rate_per_tick":2}]`)
	})
	out, err := captureStdout(t, goodsCmd().Execute)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "1 citizen = 100 people") || !strings.Contains(out, "Labor pool: 10 citizens  ·  Idle: 3 citizens") || strings.Contains(out, "200") {
		t.Fatal(out)
	}
}

func TestAIRound3_YesDoesNotAsk(t *testing.T) {
	posted := false
	round3Client(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/founding/status"):
			fmt.Fprint(w, `{"active":true,"q":0,"r":0,"population":1000,"grain":{"amount":1200}}`)
		case strings.HasSuffix(r.URL.Path, "/founding/settle"):
			posted = true
			fmt.Fprint(w, `{"tile":{"q":0,"r":0}}`)
		case strings.HasSuffix(r.URL.Path, "/provinces"):
			fmt.Fprint(w, `[]`)
		default:
			fmt.Fprint(w, `{"grain":{"base_per_tick":5,"consumption_per_tick":5}}`)
		}
	})
	cmd := foundingSettleCmd()
	cmd.SetArgs([]string{"--yes"})
	out, err := captureStdout(t, cmd.Execute)
	if err != nil || !posted || strings.Contains(out, "[y/N]") || strings.Contains(out, "Found the metropolis here?") {
		t.Fatalf("%v posted%v %s", err, posted, out)
	}
}

func TestAIRound3_ExistingStarsAndGrammar(t *testing.T) {
	out := runCityCmd(t, `{"total_gubbar":10,"pool_size":5,"hexes":[{"hex_ordinal":13,"terrain":"coastal_sea","goods":[{"good_key":"fish","placed":5}]}]}`, `{"settlement":{"food_gubbar_required":2,"food_gubbar_placed":5,"food_self_sufficient":true}}`)
	if strings.Contains(out, "could move") || strings.Contains(out, "★ food surplus") {
		t.Fatal(out)
	}
	for _, n := range []int{1, 2} {
		s := surplusWithoutSinkWarning("Timber", 10000, 2, 1, n)
		want := "citizens produce"
		if n == 1 {
			want = "citizen produces"
		}
		if !strings.Contains(s, want) {
			t.Fatal(s)
		}
	}
	if !strings.Contains(heldHexNote(placementHex{HeldBy: "Tylissos"}), "already worked by another city") {
		t.Fatal("unexplained shared hex")
	}
}

func TestAIRound3_ETAOnlyGameDays(t *testing.T) {
	c := &Client{tickSeconds: 3600, tickSecondsFetched: true}
	got := gameETA(c, time.Now().Add(90*time.Minute))
	if got != "in 2 days" {
		t.Fatal(got)
	}
	err := apiError([]byte(`{"error":"insufficient_goods","missing":[{"good_key":"stone","have":0,"need":4,"rate_per_tick":2}]}`), 422)
	if !strings.Contains(err.Error(), "if nothing else uses it") {
		t.Fatal(err)
	}
}

func TestAIRound3_ForeignWorkersAreNotRemovalTargets(t *testing.T) {
	opts := &placementOptionsResp{Hexes: []placementHex{{HexOrdinal: 1, HeldBy: "Tylissos", Goods: []placementGood{{GoodKey: "fish", Placed: 5}}}}}
	if got := occupiedGoodHint(opts, "fish"); strings.Contains(got, "#1") {
		t.Fatal(got)
	}
	opts.Hexes[0].HeldBy = ""
	opts.Hexes[0].Goods[0].Placed = 1
	if got := occupiedGoodHint(opts, "fish"); !strings.Contains(got, "(1 citizen)") {
		t.Fatal(got)
	}
	out := runCityCmd(t, `{"total_gubbar":10,"hexes":[{"hex_ordinal":1,"held_by":"Tylissos","goods":[{"good_key":"fish","placed":5}]}]}`, `{"settlement":{"food_gubbar_required":2,"food_gubbar_placed":5,"food_self_sufficient":true}}`)
	if strings.Count(out, "★") != 0 {
		t.Fatal("foreign workers marked as removable: " + out)
	}
}

func TestAIRound3_ArmyGrainIsPaidBeforePeople(t *testing.T) {
	round3Client(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/provinces/default") {
			fmt.Fprint(w, `{"settlement":{"population":1000,"grain_prod_rate":1,"grain_consum_rate":5,"army_upkeep":{"grain":0.5},"net_grain_per_tick_after_upkeep":-4.5,"net_silver_per_tick_after_upkeep":0,"food_net_per_tick_after_upkeep":0.5,"sitos":{"food_net_per_tick":1},"resources":{"grain":{"amount":0,"rate":1},"fish":{"amount":100,"rate":5}}}}`)
		} else {
			fmt.Fprint(w, `[]`)
		}
	})
	out, err := captureStdout(t, statusCmd().Execute)
	if err != nil || strings.Contains(out, "units can starve") {
		t.Fatalf("false army warning: %v %s", err, out)
	}
}
