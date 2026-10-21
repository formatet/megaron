package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestUnitCancelTrainingUsesExactUnit(t *testing.T) {
	for _, jsonOutput := range []bool{false, true} {
		oldCfg, oldJSON := cfg, jsonMode
		func() {
			defer func() { cfg, jsonMode = oldCfg, oldJSON }()
			requests := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "POST" || r.URL.Path != "/api/v1/worlds/w/units/u/cancel-training" {
					t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
				}
				requests++
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{"pop_restored":100,"refunded":{"grain":300},"already_cancelled":false}`))
			}))
			defer srv.Close()
			cfg = &Config{Server: srv.URL, WorldID: "w"}
			jsonMode = jsonOutput
			cmd := unitCancelTrainingCmd()
			if err := cmd.Flags().Set("unit", "u"); err != nil {
				t.Fatal(err)
			}
			if err := cmd.RunE(cmd, nil); err != nil {
				t.Fatal(err)
			}
			if requests != 1 {
				t.Fatalf("requests=%d", requests)
			}
		}()
	}
}
