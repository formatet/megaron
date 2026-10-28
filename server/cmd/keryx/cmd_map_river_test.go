package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMapShowsRiverFlowAndFogContinuationFromKnownHex(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if testWorldTimeResponse(w, r) {
			return
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "/provinces/home"):
			json.NewEncoder(w).Encode(map[string]any{"map_tile": map[string]int{"q": 0, "r": 0}})
		case strings.HasSuffix(r.URL.Path, "/provinces"):
			json.NewEncoder(w).Encode([]any{})
		case strings.HasSuffix(r.URL.Path, "/map"):
			w.Write([]byte(`[{"q":0,"r":0,"terrain":"river","visible":true,"tier":"remembered","river_connections":[{"edge":0,"terrain":"river_ford"}],"river_edges":[0,1],"river_flow_known":true,"river_upstream_edges":[1],"river_downstream_edges":[0]},{"q":1,"r":0,"terrain":"fog","visible":false},{"q":-1,"r":0,"terrain":"river","visible":true}]`))
		default:
			t.Errorf("unexpected %s", r.URL.Path)
		}
	}))
	defer srv.Close()
	old, j := cfg, jsonMode
	defer func() { cfg, jsonMode = old, j }()
	cfg = &Config{Server: srv.URL, Token: "token", WorldID: "world", ProvinceID: "home"}
	jsonMode = false
	cmd := mapCmd()
	out, err := captureStdout(t, cmd.Execute)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"flows from north-west toward south-east", "continues into fog toward south-east", "connections south-east: ford"} {
		if !strings.Contains(out, s) {
			t.Errorf("missing %q in %s", s, out)
		}
	}
	jsonMode = true
	cmd = mapCmd()
	out, err = captureStdout(t, cmd.Execute)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"river_downstream_edges":[0]`) {
		t.Errorf("JSON lost river metadata: %s", out)
	}
}
