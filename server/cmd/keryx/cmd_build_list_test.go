package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func buildListRows(t *testing.T, resources map[string]any) string {
	t.Helper()
	round3Client(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/api/v1/buildings") {
			_ = json.NewEncoder(w).Encode([]any{
				map[string]any{"type": "harbour", "purpose": "ships"},
				map[string]any{"type": "farm", "purpose": "grain"},
				map[string]any{"type": "storehouse", "purpose": "store"},
			})
			return
		}
		sett := map[string]any{"name": "Mycenae", "build_quotes": []any{
			map[string]any{"type": "harbour", "current_level": 0, "next_level": 1, "costs": map[string]any{"stone": 4, "timber": 3, "cedar": 2}, "duration_game_days": 3},
			map[string]any{"type": "farm", "current_level": 0, "next_level": 1, "costs": map[string]any{"timber": 1}, "duration_game_days": 2},
			map[string]any{"type": "storehouse", "current_level": 3, "next_level": nil, "at_max_level": true, "costs": map[string]any{}},
		}}
		if resources != nil {
			sett["resources"] = resources
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"settlement": sett})
	})
	out, err := captureStdout(t, func() error { return listCityBuilds(newClient(cfg), "world", "p") })
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestBuildListAfford(t *testing.T) {
	res := map[string]any{
		"stone": map[string]any{"amount": 1.0}, "timber": map[string]any{"amount": 2.0}, "cedar": map[string]any{"amount": 5.0},
	}
	out := buildListRows(t, res)
	var harbour, farm, store string
	for _, l := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(l, "harbour"):
			harbour = l
		case strings.HasPrefix(l, "farm"):
			farm = l
		case strings.HasPrefix(l, "storehouse"):
			store = l
		}
	}
	if !strings.Contains(harbour, "needs 4 stone, city has 1") || strings.Contains(harbour, "needs 3 timber") || strings.Contains(harbour, "timber,") {
		t.Fatalf("harbour row: %s", harbour)
	}
	if !strings.Contains(farm, " yes ") {
		t.Fatalf("farm row: %s", farm)
	}
	if !strings.Contains(store, "—") || strings.Contains(store, " yes ") {
		t.Fatalf("max-level row: %s", store)
	}
	if !strings.Contains(out, "· afford") {
		t.Fatalf("no afford header: %s", out)
	}
	// The row's own cedar need is not spare: cedar 4 − 2 = 2 counts as timber, short of 3.
	own := buildListRows(t, map[string]any{"stone": map[string]any{"amount": 9.0}, "cedar": map[string]any{"amount": 4.0}})
	if !strings.Contains(own, "needs 3 timber, city has 2") {
		t.Fatalf("own cedar need counted as spare: %s", own)
	}
	if unknown := buildListRows(t, nil); !strings.Contains(unknown, " ? ") {
		t.Fatalf("missing stock should print ?: %s", unknown)
	}
}
