package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLiveRemainder_ConditionalExpeditionAndTimber(t *testing.T) {
	saved := jsonMode
	jsonMode = false
	defer func() { jsonMode = saved }()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Error("preview dispatched")
		}
		w.Write([]byte(`{"available":false,"reason":"unknown_terrain","planned_expedition_ticks":12,"planned_home_by_tick_if_departing_now":17}`))
	}))
	defer srv.Close()
	out, err := captureStdout(t, func() error {
		return showMarchPreview(newClient(&Config{Server: srv.URL}), "/preview", map[string]any{})
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, part := range []string{"Planned expedition: 12 days", "with immediate departure, home by day 17", "First-leg arrival remains unknown", "No order sent"} {
		if !strings.Contains(out, part) {
			t.Fatalf("missing %q: %s", part, out)
		}
	}
	var p colonizePreview
	p.TimberWarning = "No timber within reach — every building needs timber."
	if out := captureForecastOutput(t, "Site", &p); !strings.Contains(out, p.TimberWarning) {
		t.Fatal(out)
	}
}

func TestLiveRemainder_DivineValuesAtAltar(t *testing.T) {
	out := capturePrint(t, func() { printDivineValues(map[string]float64{"bronze": 37.5, "grain": 2.5}) })
	for _, value := range []string{"Offering values", "bronze", "37.5", "grain", "2.5", "not trade prices"} {
		if !strings.Contains(out, value) {
			t.Fatal(out)
		}
	}
	if strings.Contains(strings.ToLower(out), "workday") {
		t.Fatal(out)
	}
}

func TestLiveRemainder_GoodsDoesNotPresentTechnicalValuation(t *testing.T) {
	original, mode := cfg, jsonMode
	jsonMode = false
	defer func() { cfg = original; jsonMode = mode }()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Error("goods mutated")
		}
		w.Write([]byte(`[{"key":"grain","amount":5,"rate_per_tick":1,"labor_value":11,"producible":true}]`))
	}))
	defer srv.Close()
	cfg = &Config{Server: srv.URL, Token: "token", WorldID: "w", ProvinceID: "p"}
	out, err := captureStdout(t, func() error { cmd := goodsCmd(); cmd.SetArgs([]string{}); return cmd.Execute() })
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "grain") || strings.Contains(strings.ToLower(out), "workday") {
		t.Fatal(out)
	}
}
