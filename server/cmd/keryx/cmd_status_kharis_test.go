package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestStatusRealmKharisDecay(t *testing.T) {
	for _, tc := range []struct {
		name                        string
		cities, base, burden, total float64
		want                        string
	}{
		{"capital without temple", 1, .1, 0, .35, "0.10 base + 1 city cost 0.00 + 2500 residents cost 0.25 = 0.35 kharis per day"},
		{"five cities", 5, .1, 1, 1.35, "0.10 base + 5 cities cost 1.00 + 2500 residents cost 0.25 = 1.35 kharis per day"},
		{"server values rather than client formula", 5, .7, 8.6, 9.55, "0.70 base + 5 cities cost 8.60 + 2500 residents cost 0.25 = 9.55 kharis per day"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			round3Client(t, func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/provinces/default") {
					_ = json.NewEncoder(w).Encode(map[string]any{"settlement": map[string]any{
						"name": "Mycenae", "population": 1000, "labor_pool": 1000, "resources": map[string]any{},
						"kharis": 40, "kharis_cap": 40, "kharis_mood": "Indifferent", "kharis_per_tick": 0,
						"kharis_net_known": true, "kharis_net_per_tick": -tc.total,
						"kharis_population": 2500, "kharis_population_decay": .25, "kharis_production_per_tick": 2.1, "kharis_per_temple_worker": 1.05, "kharis_city_count": tc.cities, "kharis_base_decay": tc.base, "kharis_city_decay": tc.burden, "kharis_daily_decay": tc.total,
						"temple_offers": []any{},
					}})
				} else {
					_ = json.NewEncoder(w).Encode([]any{})
				}
			})
			out, err := captureStdout(t, statusCmd().Execute)
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{tc.want, "net -", "ceiling 40", "Temples: none", "Temple production: 2.10", "worker gives at least +1.05", "grain is scarce in the world"} {
				if !strings.Contains(out, want) {
					t.Fatalf("missing %q: %s", want, out)
				}
			}
			for _, bad := range []string{"passive +0.0", "holds for", "144 days"} {
				if strings.Contains(out, bad) {
					t.Fatalf("obsolete hint %q: %s", bad, out)
				}
			}
		})
	}
}

func TestKharisDecayLineUnavailable(t *testing.T) {
	for _, sett := range []map[string]any{nil, {}, {"kharis_city_count": 5.0}} {
		if got := kharisDecayLine(sett); got != "Cult upkeep not reported" {
			t.Fatal(got)
		}
	}
}

func TestStatusDivineProtectionAndBlessingRequirement(t *testing.T) {
	round3Client(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/provinces/default") {
			json.NewEncoder(w).Encode(map[string]any{"settlement": map[string]any{
				"name": "Pylos", "population": 1000, "resources": map[string]any{}, "kharis_blessing_threshold": 60,
				"kharis_wrath": map[string]any{"threshold": 15, "daily_probability": .03, "protection_remaining_days": 2, "protection_until": "2026-10-23T18:30:00Z"},
			}})
			return
		}
		json.NewEncoder(w).Encode([]any{})
	})
	out, err := captureStdout(t, statusCmd().Execute)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Everyday prayers work at a level-1 temple", "need 60 kharis", "Only a level-2 temple or higher", "Kharis retained above a lowered ceiling still counts", "with or without a temple", "Protected from divine punishment until"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q: %s", want, out)
		}
	}
}

func TestStatusAboveCeilingIgnoresTempleGain(t *testing.T) {
	round3Client(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/provinces/default") {
			json.NewEncoder(w).Encode(map[string]any{"settlement": map[string]any{"name": "Pylos", "population": 1000, "resources": map[string]any{}, "kharis": 80, "kharis_cap": 50, "kharis_net_known": true, "kharis_net_per_tick": -.2, "kharis_per_temple_worker": 1.05}})
			return
		}
		json.NewEncoder(w).Encode([]any{})
	})
	out, err := captureStdout(t, statusCmd().Execute)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"until it is below the ceiling", "upkeep only above ceiling", "net -0.2"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q: %s", want, out)
		}
	}
	if strings.Contains(out, "One more fed temple worker") {
		t.Fatal(out)
	}
}
