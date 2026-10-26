package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestResolveUnitExactDisplayName(t *testing.T) {
	for _, duplicate := range []bool{false, true} {
		t.Run(map[bool]string{false: "unique", true: "ambiguous"}[duplicate], func(t *testing.T) {
			body := `{"units":[{"id":"aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa","display_name":"3rd Spearmen of Tylissos"}`
			if duplicate {
				body += `,{"id":"bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb","display_name":"3rd Spearmen of Tylissos"}`
			}
			body += `]}`
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(body)) }))
			defer srv.Close()
			id, err := resolveUnitID(newClient(&Config{Server: srv.URL}), "world", " 3RD SPEARMEN OF TYLISSOS ")
			if duplicate {
				if err == nil || !strings.Contains(err.Error(), "matches 2") {
					t.Fatalf("ambiguous name must not guess: %s %v", id, err)
				}
			} else if err != nil || id != "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa" {
				t.Fatalf("named unit not resolved: %s %v", id, err)
			}
		})
	}
}

func TestExploreRefusalBeforeRunnerDispatch(t *testing.T) {
	posts, previews := 0, 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/cities") {
			w.Write([]byte(`[]`))
			return
		}
		if strings.HasSuffix(r.URL.Path, "/march-preview") {
			previews++
			w.WriteHeader(422)
			w.Write([]byte(`{"error":"the chosen expedition duration is too short"}`))
			return
		}
		if r.Method == "POST" {
			posts++
			w.WriteHeader(202)
			w.Write([]byte(`{"status":"order_dispatched"}`))
			return
		}
		if testWorldTimeResponse(w, r) {
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	oldCfg, oldJSON := cfg, jsonMode
	defer func() { cfg, jsonMode = oldCfg, oldJSON }()
	cfg = &Config{Server: srv.URL, WorldID: "world"}
	jsonMode = true
	cmd := unitMarchCmd()
	cmd.SetArgs([]string{"--unit", "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "--target", "10,0", "--intent", "explore", "--ticks", "6"})
	_, err := captureStdout(t, cmd.Execute)
	if err == nil || !strings.Contains(err.Error(), "too short") || previews != 1 || posts != 0 {
		t.Fatalf("preflight must refuse before sending: err%v previews%d posts%d", err, previews, posts)
	}
}
