package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestKeryxRiverNamesMap(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/map"):
			w.Write([]byte(`[{"q":1,"r":0,"terrain":"river","river_name":"Alpheios","visible":true,"tier":"live"},{"q":2,"r":0,"terrain":"river_ford","river_name":"Alpheios","visible":true,"tier":"remembered"},{"q":3,"r":0,"terrain":"river_delta","river_name":"Alpheios","visible":true,"tier":"live"}]`))
		case strings.HasSuffix(r.URL.Path, "/provinces"):
			w.Write([]byte(`[]`))
		default:
			w.Write([]byte(`{"map_tile":{"q":0,"r":0}}`))
		}
	}))
	defer ts.Close()
	old, oldJSON := cfg, jsonMode
	defer func() { cfg, jsonMode = old, oldJSON }()
	cfg = &Config{Server: ts.URL, WorldID: "world", ProvinceID: "city", Token: "t"}
	jsonMode = false
	oldOut := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() { os.Stdout = oldOut }()
	cmd := mapCmd()
	cmd.SetArgs([]string{})
	e := cmd.Execute()
	w.Close()
	var out bytes.Buffer
	out.ReadFrom(r)
	r.Close()
	if e != nil {
		t.Fatal(e)
	}
	for _, want := range []string{"River Alpheios", "Ford of Alpheios", "Delta of Alpheios"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("missing %s in %s", want, out.String())
		}
	}
}
func TestKeryxRiverNamesCatchment(t *testing.T) {
	var h placementHex
	if e := json.Unmarshal([]byte(`{"terrain":"river_ford","river_name":"Alpheios"}`), &h); e != nil {
		t.Fatal(e)
	}
	if got := namedRiverTerrain(h.Terrain, h.RiverName); got != "Ford of Alpheios" {
		t.Fatal(got)
	}
	if namedRiverTerrain("plains", "Alpheios") != "plains" {
		t.Fatal("named nonriver")
	}
}

func TestKeryxRiverNameOnceOnBuildingCatchmentRow(t *testing.T) {
	out := runCityCmd(t, `{"hexes":[{"hex_q":2,"hex_r":3,"hex_ordinal":1,"terrain":"river_ford","river_name":"Eurotas","building":{"type":"farm","level":1},"goods":[]}]}`, `{}`)
	if strings.Count(out, "Eurotas") != 1 {
		t.Fatalf("river name repeated in catchment row: %s", out)
	}
	if !strings.Contains(out, "Ford of Eurotas") || !strings.Contains(out, "farm L1 @ (2,3)") {
		t.Fatalf("missing river/building identity: %s", out)
	}
}
