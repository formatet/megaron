package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCitiesListsAllKnownDepositsIncludingIncense(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/worlds/world/cities" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`[{"name":"Known Source","settlement_id":"known-id","knowledge":"known","copper_deposit":true,"cedar_deposit":true,"incense_deposit":true},{"name":"Distant","knowledge":"rumour","bearing":"east","industry_hint":"stone","incense_deposit":true,"settlement_id":"secret-id"}]`))
	}))
	defer ts.Close()
	old, oldJSON := cfg, jsonMode
	defer func() { cfg, jsonMode = old, oldJSON }()
	cfg = &Config{Server: ts.URL, WorldID: "world"}
	jsonMode = false
	out, err := captureStdout(t, func() error { cmd := citiesCmd(); return cmd.RunE(cmd, nil) })
	if err != nil {
		t.Fatal(err)
	}
	known := strings.Split(out, "RUMOUR-KNOWN")[0]
	for _, want := range []string{"⛏copper", "cedar", "incense", "known-id"} {
		if !strings.Contains(known, want) {
			t.Errorf("known city missing%s: %s", want, out)
		}
	}
	rumour := strings.Split(out, "RUMOUR-KNOWN")[1]
	if strings.Contains(rumour, "secret-id") || strings.Contains(rumour, "incense") {
		t.Fatalf("rumour leaks exact deposit or ID: %s", rumour)
	}
}
