package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHexInspectsPhysicalOwnAndOnlyPublicForeignFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if testWorldTimeResponse(w, r) {
			return
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "/map"):
			json.NewEncoder(w).Encode([]map[string]any{{"q": 2, "r": 0, "terrain": "plains"}, {"q": 0, "r": 0, "terrain": "plains"}})
		case strings.HasSuffix(r.URL.Path, "/foreign-units"):
			w.Write([]byte(`[{"id":"foreign","name":"Seen ship","owner":"Other","type":"galley","category":"naval","size":1,"status":"marching","q":2,"r":0,"heading":"south","hull":3,"target_q":999,"cargo":{"type":"spearman","size":100,"units":1}}]`))
		case strings.HasSuffix(r.URL.Path, "/trades"):
			w.Write([]byte(`[]`))
		case strings.HasSuffix(r.URL.Path, "/units"):
			w.Write([]byte(`{"units":[{"id":"own","display_name":"Marching force","type":"spearman","category":"land","size":100,"status":"marching","q":0,"r":0,"current_q":2,"current_r":0,"target_q":4,"target_r":0},{"id":"ship","display_name":"Own ship","type":"merchantman","category":"naval","size":1,"hull":5,"status":"positioned","q":2,"r":0,"provision_days":4,"cargo_units":[{"display_name":"Aboard force","type":"spearman","size":100}]}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	old, j := cfg, jsonMode
	defer func() { cfg, jsonMode = old, j }()
	cfg = &Config{Server: srv.URL, Token: "token", WorldID: "world"}
	jsonMode = false
	cmd := hexCmd()
	cmd.SetArgs([]string{"2,0"})
	out, err := captureStdout(t, cmd.Execute)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Marching force", "Own ship", "Hull 5/5", "Aboard force", "Seen ship", "Hull: unknown", "keryx unit redirect", "messenger"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q: %s", want, out)
		}
	}
	if strings.Contains(out, "999") || strings.Contains(out, "Hull 3") {
		t.Fatalf("foreign field leaked: %s", out)
	}
	jsonMode = true
	cmd = hexCmd()
	cmd.SetArgs([]string{"2,0"})
	out, err = captureStdout(t, cmd.Execute)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]json.RawMessage
	if err = json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got["foreign"]), "hull") || strings.Contains(string(got["foreign"]), "999") {
		t.Fatal("private foreign fields reached JSON")
	}
	jsonMode = false
	cmd = hexCmd()
	cmd.SetArgs([]string{"0,0"})
	out, err = captureStdout(t, cmd.Execute)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "Marching force") {
		t.Fatal("march matched origin instead of current position")
	}
}
