package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAIRound_NearZeroForecast(t *testing.T) {
	if got := formatNetPerTick(-1e-12); strings.Contains(got, "-0.0") {
		t.Fatal(got)
	}
	p := &colonizePreview{}
	p.Grain.BasePerTick = 5
	p.Grain.EstNetPerTick = -1e-12
	p.Grain.Seed = 1200
	days := 1e18
	p.Grain.TicksUntilEmpty = &days
	out := captureForecastOutput(t, "Test", p)
	if strings.Contains(out, "1000000000000000000") || strings.Contains(out, "-0.0") {
		t.Fatal(out)
	}
}

func TestAIRound_FarmAlreadyIncluded(t *testing.T) {
	p := &colonizePreview{}
	p.Grain.BasePerTick = 4.67
	p.Grain.WithFarmPerTick = 4.67
	p.Grain.EstNetPerTick = -0.33
	p.Grain.StarterFarmIncluded = true
	possible := true
	p.Grain.FarmPossible = &possible
	out := captureForecastOutput(t, "Tylissos", p)
	if strings.Contains(out, "no farmable") || !strings.Contains(out, "already included") {
		t.Fatal(out)
	}
}

func TestAIRound_WorldSelectionAndPreserveConfig(t *testing.T) {
	for _, multi := range []bool{false, true} {
		t.Run(fmt.Sprint(multi), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/v1/worlds":
					if multi {
						fmt.Fprint(w, `[{"id":"one","state":"active"},{"id":"two","state":"active"}]`)
					} else {
						fmt.Fprint(w, `[{"id":"one","state":"active"}]`)
					}
				case "/api/v1/worlds/one/provinces":
					fmt.Fprint(w, `[{"id":"city","own":true}]`)
				default:
					fmt.Fprint(w, `{"settlement":{"name":"Test"}}`)
				}
			}))
			defer srv.Close()
			path := filepath.Join(t.TempDir(), "config.json")
			data, _ := json.Marshal(map[string]string{"server": srv.URL, "token": "test-token", "keep": "opaque"})
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("POLEIA_CONFIG", path)
			t.Setenv("POLEIA_SERVER", "")
			t.Setenv("POLEIA_TOKEN", "")
			old, oldJSON := cfg, jsonMode
			defer func() { cfg, jsonMode = old, oldJSON }()
			root := newRootCommand()
			root.SetArgs([]string{"status", "--json"})
			_, err := captureStdout(t, root.Execute)
			if multi {
				if err == nil || !strings.Contains(err.Error(), "--select") {
					t.Fatal(err)
				}
				root = newRootCommand()
				root.SetArgs([]string{"worlds", "--select", "one"})
				if err := root.Execute(); err != nil {
					t.Fatal(err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			saved, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var obj map[string]any
			_ = json.Unmarshal(saved, &obj)
			if obj["world_id"] != "one" || obj["keep"] != "opaque" {
				t.Fatal(string(saved))
			}
		})
	}
}

func TestAIRound_ColonizeConfirmationAndHost(t *testing.T) {
	const id = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	for _, host := range []bool{false, true} {
		t.Run(fmt.Sprint(host), func(t *testing.T) {
			writes := 0
			input, err := os.CreateTemp(t.TempDir(), "stdin")
			if err != nil {
				t.Fatal(err)
			}
			oldInput := os.Stdin
			os.Stdin = input
			defer func() { os.Stdin = oldInput; input.Close() }()
			kind := "spearman"
			if host {
				kind = "nomadic_host"
			}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" {
					writes++
				}
				if strings.HasSuffix(r.URL.Path, "/units") {
					fmt.Fprintf(w, `{"units":[{"id":%q,"type":%q}]}`, id, kind)
				} else {
					fmt.Fprint(w, `{"grain":{"base_per_tick":5,"est_net_per_tick":0}}`)
				}
			}))
			defer srv.Close()
			old, oldJSON := cfg, jsonMode
			defer func() { cfg, jsonMode = old, oldJSON }()
			cfg = &Config{Server: srv.URL, Token: "token", WorldID: "world"}
			jsonMode = false
			cmd := unitCmd()
			cmd.SetArgs([]string{"march", "--unit", id, "--target", "5,0", "--intent", "colonize"})
			out, err := captureStdout(t, cmd.Execute)
			if host {
				if err == nil || !strings.Contains(err.Error(), "founding settle") {
					t.Fatalf("%s %v", out, err)
				}
			} else if err != nil || !strings.Contains(out, "Nothing sent") {
				t.Fatalf("%s %v", out, err)
			}
			if writes != 0 {
				t.Fatal("unconfirmed order sent")
			}
		})
	}
}
func TestAIRound_ShortfallProductionWait(t *testing.T) {
	err := apiError([]byte(`{"error":"insufficient_goods","missing":[{"good":"stone","need":4,"have":2,"rate_per_tick":2}]}`), 422)
	if !strings.Contains(err.Error(), "≈1 day at current production") {
		t.Fatal(err)
	}
	no := apiError([]byte(`{"error":"insufficient_goods","missing":[{"good":"stone","need":4,"have":2,"rate_per_tick":0}]}`), 422)
	if strings.Contains(no.Error(), "production") {
		t.Fatal(no)
	}
}

func TestAIRound_StatusUsesFoodAndShowsGranary(t *testing.T) {
	for _, net := range []float64{0.5, -2.5} {
		t.Run(fmt.Sprint(net), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/v1/worlds/world/provinces/city" {
					fmt.Fprintf(w, `{"settlement":{"name":"Coast","population":1000,"labor_pool":1000,"grain_prod_rate":0.5,"grain_consum_rate":5,"resources":{"grain":{"amount":10000,"rate":0.5},"fish":{"amount":10,"rate":5},"timber":{"amount":1,"rate":1}},"sitos":{"food_net_per_tick":%g,"granary_per_good":{"grain":307}}}}`, net)
				} else {
					fmt.Fprint(w, `[]`)
				}
			}))
			defer srv.Close()
			old, oldJSON := cfg, jsonMode
			defer func() { cfg, jsonMode = old, oldJSON }()
			cfg = &Config{Server: srv.URL, Token: "token", WorldID: "world", ProvinceID: "city"}
			jsonMode = false
			cmd := statusCmd()
			out, err := captureStdout(t, cmd.Execute)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out, "grain 0.5 + fish 5.0") || !strings.Contains(out, "net "+formatNetPerTick(net)) || !strings.Contains(out, "307 grain in Sitos") {
				t.Fatal(out)
			}
			if net < 0 && strings.Contains(out, "Grain    10000  production") {
				t.Fatal(out)
			}
		})
	}
}

func TestAIRound_CityCoordinatesAndExploreArea(t *testing.T) {
	out := runCityCmd(t, `{"population":1000,"hexes":[{"hex_ordinal":2,"hex_q":57,"hex_r":54,"terrain":"plains","goods":[]}]}`, `{"settlement":{}}`)
	if !strings.Contains(out, "(57,54)") || !strings.Contains(out, "#2") {
		t.Fatal(out)
	}
	intent := "explore"
	q, r := 27, 36
	tq, tr := 26, 33
	u := unitRow{Status: "marching", MarchIntent: &intent, Q: &q, R: &r, TargetQ: &tq, TargetR: &tr, Expedition: &expeditionMission{AreaQ: 31, AreaR: 33}}
	loc := locationStr(nil, u, nil)
	if !strings.Contains(loc, "unseen ground near (31,33)") || strings.Contains(loc, "(26,33)") {
		t.Fatal(loc)
	}
}
