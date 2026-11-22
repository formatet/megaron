package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func statusBriefOutput(t *testing.T, args ...string) string {
	t.Helper()
	round3Client(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/provinces/default"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"catchment_deposits": []any{"copper"},
				"settlement": map[string]any{
					"name": "Mycenae", "culture": "achaean", "id": "s1", "population": 1000, "labor_pool": 1000, "walls": 1,
					"population_cap": 2000, "silo_level": 1,
					"resources": map[string]any{
						"silver": map[string]any{"amount": 120, "rate": 1},
						"timber": map[string]any{"amount": 0, "rate": 0},
						"grain":  map[string]any{"amount": 50, "rate": 1},
					},
					"automatic_food":       map[string]any{"nearjord_grain_per_tick": 1.0},
					"food_gubbar_required": 2, "food_gubbar_placed": 2, "food_self_sufficient": true,
					"food":            map[string]any{"production_per_tick": 1.0, "consumption_per_tick": 1.0, "food_net_per_tick": 0.0},
					"grain_prod_rate": 1.0, "grain_consum_rate": 1.0,
					"kharis": 1.0, "kharis_cap": 40, "kharis_mood": "Wrathful", "kharis_net_known": true, "kharis_net_per_tick": -0.4,
					"kharis_standing":   map[string]any{"short": "Kharis 1.0 · Wrathful (Suspicious at 5)"},
					"kharis_city_count": 1.0, "kharis_base_decay": 0.1, "kharis_city_decay": 0.0, "kharis_daily_decay": 0.35,
					"kharis_production_per_tick": 2.1, "kharis_blessing_threshold": 30,
					"temple_offers": []any{},
					"army":          map[string]any{"Spearman": 200},
					"army_upkeep":   map[string]any{"grain": 2, "silver": 1},
					"buildings":     []any{map[string]any{"type": "harbour", "level": 1}},
					"build_queue":   []any{},
				},
			})
		case strings.HasSuffix(r.URL.Path, "/units"):
			_ = json.NewEncoder(w).Encode(map[string]any{"units": []any{
				map[string]any{"id": "11111111-aaaa-bbbb-cccc-000000000001", "type": "spearman", "status": "garrison", "settlement_id": "s1", "size": 100},
				map[string]any{"id": "22222222-aaaa-bbbb-cccc-000000000002", "type": "spearman", "status": "garrison", "settlement_id": "s1", "size": 100},
			}})
		default:
			_ = json.NewEncoder(w).Encode([]any{})
		}
	})
	cmd := statusCmd()
	cmd.SetArgs(args)
	out, err := captureStdout(t, cmd.Execute)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestStatusBrief(t *testing.T) {
	full := statusBriefOutput(t)
	brief := statusBriefOutput(t, "--brief")
	for _, want := range []string{"⚠ Unmined deposit", "⚠ Timber production", "Kharis 1.0 · Wrathful (Suspicious at 5)", "Silver", "Spearmen", "Full detail: keryx status"} {
		if !strings.Contains(brief, want) {
			t.Fatalf("brief lacks %q:\n%s", want, brief)
		}
	}
	for _, bad := range []string{"Automatic food", "Growth", "Temples", "11111111-aaaa", "1 citizen = 100 people", "Field forces", "Buildings", "Temple production", "Wrath risk"} {
		if strings.Contains(brief, bad) {
			t.Fatalf("brief should not contain %q:\n%s", bad, brief)
		}
		if bad != "Temples" && bad != "Wrath risk" && !strings.Contains(full, bad) {
			t.Fatalf("fixture broken: full lacks %q:\n%s", bad, full)
		}
	}
	fl, bl := strings.Count(full, "\n"), strings.Count(brief, "\n")
	if bl*2 > fl {
		t.Fatalf("brief has %d lines, full %d (want at most half)", bl, fl)
	}
}
