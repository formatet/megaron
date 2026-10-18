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
		{"capital without temple", 1, .1, 0, .1, "0.1 base + 1 city cost 0.0 per day = 0.1 total"},
		{"five cities", 5, .1, 1, 1.1, "0.1 base + 5 cities cost 1.0 per day = 1.1 total"},
		{"server values rather than client formula", 5, .7, 8.6, 9.3, "0.7 base + 5 cities cost 8.6 per day = 9.3 total"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			round3Client(t, func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/provinces/default") {
					_ = json.NewEncoder(w).Encode(map[string]any{"settlement": map[string]any{
						"name": "Mycenae", "population": 1000, "labor_pool": 1000, "resources": map[string]any{},
						"kharis": 40, "kharis_cap": 40, "kharis_mood": "Indifferent", "kharis_per_tick": 0,
						"kharis_net_known": true, "kharis_net_per_tick": -tc.total,
						"kharis_city_count": tc.cities, "kharis_base_decay": tc.base, "kharis_city_decay": tc.burden, "kharis_daily_decay": tc.total,
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
			for _, want := range []string{tc.want, "net -", "cap 40", "Temples: none"} {
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
		if got := kharisDecayLine(sett); got != "Kharis decay not reported" {
			t.Fatal(got)
		}
	}
}
