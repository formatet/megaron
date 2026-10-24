package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTransferManifestPairsAndCargo(t *testing.T) {
	items, err := parseTransferItems([]string{"grain", "timber"}, []string{"12", "9"})
	if err != nil {
		t.Fatal(err)
	}
	cap := 20.0
	line := transferCargoLine(items, cargoPreview{Category: "naval", ShipName: "Naia", Capacity: &cap})
	if line != "Naia · Cargo 21 / 20 — remove 1 goods" {
		t.Fatal(line)
	}
	if got := transferItemText(items); got != "12 grain, 9 timber" {
		t.Fatal(got)
	}
	if got := transferCargoLine(items, cargoPreview{Category: "land"}); !strings.Contains(got, "no cargo limit") {
		t.Fatal(got)
	}
	for _, q := range [][]string{{"1"}, {"1", "NaN"}, {"1", "-1"}, {"1", "+Inf"}} {
		if _, err := parseTransferItems([]string{"grain", "timber"}, q); err == nil {
			t.Fatal(q)
		}
	}
}
func TestManifestNotificationKeepsEveryAmount(t *testing.T) {
	out, err := captureStdout(t, func() error {
		printManifestOutcome(notificationItem{Kind: "GiftDeliveredV2", Body: []byte(`{"sender_name":"Myrine","recipient_name":"Leandros","origin_name":"Secret city","items":[{"good_key":"grain","credited_quantity":6,"lost_quantity":6},{"good_key":"timber","credited_quantity":4,"lost_quantity":4}]}`)})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Myrine → Leandros", "6 grain received; 6 lost", "4 timber received; 4 lost"} {
		if !strings.Contains(out, want) {
			t.Fatal(out)
		}
	}
	if strings.Contains(out, "Secret city") {
		t.Fatal(out)
	}
}

func TestTransferRepeatedFlagsSendOneManifest(t *testing.T) {
	const dest = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	posts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if testWorldTimeResponse(w, r) {
			return
		}
		if strings.HasSuffix(r.URL.Path, "/preview") {
			json.NewEncoder(w).Encode(map[string]any{"category": "naval", "ship_name": "Naia", "cargo_capacity": 20})
			return
		}
		posts++
		var body struct {
			Items []transferItem `json:"items"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if len(body.Items) != 2 || body.Items[0].GoodKey != "grain" || body.Items[1].GoodKey != "timber" || body.Items[1].Quantity != 8 {
			t.Errorf("wrong body %+v", body)
		}
		json.NewEncoder(w).Encode(map[string]any{"kind": "transfer", "ship_name": "Naia", "arrival_tick": 42, "travel_ticks": 7})
	}))
	defer srv.Close()
	old, mode := cfg, jsonMode
	defer func() { cfg, jsonMode = old, mode }()
	cfg = &Config{Server: srv.URL, WorldID: "world", ProvinceID: "source"}
	jsonMode = false
	cmd := transferCmd()
	cmd.SetArgs([]string{"--dest", dest, "--good", "grain", "--qty", "12", "--good", "timber", "--qty", "8"})
	out, err := captureStdout(t, cmd.Execute)
	if err != nil {
		t.Fatal(err)
	}
	if posts != 1 || !strings.Contains(out, "Cargo 20 / 20") || !strings.Contains(out, "12 grain, 8 timber") {
		t.Fatalf("posts%d %s", posts, out)
	}
}
