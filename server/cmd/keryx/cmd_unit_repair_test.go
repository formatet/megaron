package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUnitRepairResolvesNameAndShortInput(t *testing.T) {
	const id = "8afb6a29-1111-2222-3333-444444444444"
	var posted string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/units"):
			_, _ = w.Write([]byte(`{"units":[{"id":"` + id + `","display_name":"Wavecutter","type":"ship"}]}`))
		case r.Method == http.MethodPost:
			posted = r.URL.Path
			_, _ = w.Write([]byte(`{"hull_before":3,"hull_target":6,"good":"timber","amount":4,"duration_ticks":2,"shipyard_waiting":true}`))
		}
	}))
	defer ts.Close()
	old, oldJSON := cfg, jsonMode
	defer func() { cfg, jsonMode = old, oldJSON }()
	cfg = &Config{Server: ts.URL, WorldID: "world"}
	jsonMode = false
	for _, in := range []string{"Wavecutter", "8af", "ab"} {
		posted = ""
		cmd := unitRepairCmd()
		_ = cmd.Flags().Set("unit", in)
		_, err := captureStdout(t, func() error { return cmd.RunE(cmd, nil) })
		if in == "ab" {
			if err == nil {
				t.Fatalf("short non-matching input must be an error, not a panic")
			}
			continue
		}
		if err != nil {
			t.Fatalf("%s: %v", in, err)
		}
		if posted != "/api/v1/worlds/world/units/"+id+"/repair" {
			t.Fatalf("%s: posted to %s", in, posted)
		}
	}
}
