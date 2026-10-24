package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHarvestBlessingBothCityCommands(t *testing.T) {
	for _, tc := range []struct {
		name, b string
		active  bool
	}{
		{"active", `{"remaining_ticks":12,"yield_bonus":0.37,"goods":["grain","wine"]}`, true},
		{"end tick", `{"remaining_ticks":0,"yield_bonus":0.37,"goods":["grain","wine"]}`, false},
		{"expired", `{"remaining_ticks":-1,"yield_bonus":0.37,"goods":["grain","wine"]}`, false},
		{"none", `null`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := fmt.Sprintf(`{"settlement":{"name":"Mycenae","population":1000,"food_gubbar_required":7,"food_gubbar_placed":7,"harvest_blessing":%s}}`, tc.b)
			cityOut := runCityCmd(t, `{"hexes":[],"buildings":[],"total_gubbar":10,"pool_size":3}`, body)
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/provinces/prov-1") {
					fmt.Fprint(w, body)
				} else {
					fmt.Fprint(w, `{}`)
				}
			}))
			defer ts.Close()
			oldCfg, oldJSON := cfg, jsonMode
			cfg = &Config{Server: ts.URL, WorldID: "world-1", ProvinceID: "prov-1", Token: "t"}
			jsonMode = false
			defer func() { cfg = oldCfg; jsonMode = oldJSON }()
			statusOut, err := captureStdout(t, func() error { return statusCmd().RunE(nil, nil) })
			if err != nil {
				t.Fatal(err)
			}
			for name, out := range map[string]string{"city": cityOut, "status": statusOut} {
				if tc.active {
					want := "Blessed harvest: +37% grain, wine yield · 12 game days left"
					if !strings.Contains(out, want) {
						t.Errorf("%s missing authoritative harvest row: %s", name, out)
					}
				} else if strings.Contains(out, "Blessed harvest") {
					t.Errorf("%s shows ended harvest: %s", name, out)
				}
			}
		})
	}
}
