package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

const (
	llmSettlementUUID = "aaaaaaaa-1111-2222-3333-444444444444"
	llmShipUUID       = "bbbbbbbb-1111-2222-3333-444444444444"
	llmLandUUID       = "cccccccc-1111-2222-3333-444444444444"
)

// llmServer answers the lookups the name resolvers make and records every
// non-GET request path.
func llmServer(t *testing.T, posts *[]string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if testWorldTimeResponse(w, r) {
			return
		}
		if r.Method != "GET" {
			*posts = append(*posts, r.URL.Path)
			w.Write([]byte(`{"message":"ok","unit_id":"` + llmShipUUID + `","shore_q":1,"shore_r":2,"arrival_tick":5,"wait_ticks":3,"cargo_unit_ids":["x"]}`))
			return
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "/provinces"):
			w.Write([]byte(`[{"id":"p1","name":"Pylos","settlement_id":"` + llmSettlementUUID + `","q":4,"r":-2}]`))
		case strings.HasSuffix(r.URL.Path, "/cities"):
			w.Write([]byte(cityTargetsFixture))
		case strings.HasSuffix(r.URL.Path, "/units"):
			w.Write([]byte(`{"units":[{"id":"` + llmShipUUID + `","display_name":"Sea Swallow","type":"ship"},{"id":"` + llmLandUUID + `","display_name":"Pylos Spears","type":"infantry"}]}`))
		default:
			w.Write([]byte(`{}`))
		}
	}))
	t.Cleanup(srv.Close)
	oldCfg, oldJSON := cfg, jsonMode
	cfg = &Config{Server: srv.URL, WorldID: "w"}
	jsonMode = false
	t.Cleanup(func() { cfg, jsonMode = oldCfg, oldJSON })
	return srv
}

func runLLM(t *testing.T, cmd *cobra.Command, args ...string) (string, error) {
	t.Helper()
	cmd.SetArgs(args)
	return captureStdout(t, cmd.Execute)
}

func TestAbandonResolvesCityName(t *testing.T) {
	var posts []string
	llmServer(t, &posts)
	if _, err := runLLM(t, abandonCmd(), "--settlement", "pylos"); err != nil {
		t.Fatal(err)
	}
	want := "/api/v1/worlds/w/settlements/" + llmSettlementUUID + "/abandon"
	if len(posts) != 1 || posts[0] != want {
		t.Fatalf("posts=%v want %s", posts, want)
	}
}

func TestOccupationResolvesCityName(t *testing.T) {
	var posts []string
	llmServer(t, &posts)
	if _, err := runLLM(t, occupationOrderCmd(), "--settlement", "Pylos", "--action", "annex"); err != nil {
		t.Fatal(err)
	}
	want := "/api/v1/worlds/w/settlements/" + llmSettlementUUID + "/occupation-order"
	if len(posts) != 1 || posts[0] != want {
		t.Fatalf("posts=%v want %s", posts, want)
	}
}

func TestCancelTrainingResolvesUnitName(t *testing.T) {
	var posts []string
	llmServer(t, &posts)
	if _, err := runLLM(t, unitCancelTrainingCmd(), "--unit", "pylos spears"); err != nil {
		t.Fatal(err)
	}
	want := "/api/v1/worlds/w/units/" + llmLandUUID + "/cancel-training"
	if len(posts) != 1 || posts[0] != want {
		t.Fatalf("posts=%v want %s", posts, want)
	}
}

func TestUnitPostToCitySendsItsCoordinates(t *testing.T) {
	var posts []string
	llmServer(t, &posts)
	out, err := runLLM(t, unitPostCmd(), "--unit", "pylos spears", "--to", "Pylos")
	if err != nil || len(posts) != 1 || !strings.Contains(out, "(4,-2)") {
		t.Fatalf("%v posts=%v out=%s", err, posts, out)
	}
	// Both forms at once is refused client-side, nothing is sent.
	posts = nil
	if _, err := runLLM(t, unitPostCmd(), "--unit", "pylos spears", "--to", "Pylos", "--q", "1", "--r", "2"); err == nil || len(posts) != 0 {
		t.Fatalf("both --to and --q accepted: err=%v posts=%v", err, posts)
	}
}

func TestDispatchConfirmationUsesNameAndShortIDNeverPanics(t *testing.T) {
	var posts []string
	llmServer(t, &posts)
	out, err := runLLM(t, unitLoadCmd(), "--ship", "sea swallow", "--unit", "pylos spears")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Unit Pylos Spears embarked on ship Sea Swallow") {
		t.Fatalf("no display names in confirmation: %s", out)
	}
	if got := unitLabel("abcde"); got != "abcde" {
		t.Fatalf("short id: %q", got)
	}
	if got := unitLabel("12345678-aaaa"); got != "12345678…" {
		t.Fatalf("long id: %q", got)
	}
}

func TestSeaVerbsPointAtStorms(t *testing.T) {
	var posts []string
	llmServer(t, &posts)
	for name, run := range map[string]func() (string, error){
		"load":   func() (string, error) { return runLLM(t, unitLoadCmd(), "--ship", "sea swallow", "--unit", "pylos spears") },
		"unload": func() (string, error) { return runLLM(t, unitUnloadCmd(), "--ship", "sea swallow") },
		"pickup": func() (string, error) { return runLLM(t, unitPickupCmd(), "pylos spears", "--ship", "sea swallow") },
		"passage": func() (string, error) {
			return runLLM(t, passageCmd(), "--id", "m1", "--ship", llmShipUUID)
		},
	} {
		out, err := run()
		if err != nil || !strings.Contains(out, "keryx storms") {
			t.Errorf("%s: err=%v out=%s", name, err, out)
		}
	}
}

func TestApiErrorRefusalHints(t *testing.T) {
	err := apiError([]byte(`{"error":"no passable route for this caravan"}`), 422)
	if !strings.Contains(err.Error(), "keryx map --radius 12") || !strings.Contains(err.Error(), "keryx storms") {
		t.Fatalf("no route hint: %v", err)
	}
	for _, msg := range []string{"This ship carries no passengers", "Choose a ship docked in Pylos"} {
		err := apiError([]byte(fmt.Sprintf(`{"error":%q}`, msg)), 422)
		if !strings.Contains(err.Error(), "keryx unit list") {
			t.Fatalf("no ship hint for %q: %v", msg, err)
		}
	}
	plain := apiError([]byte(`{"error":"not enough grain"}`), 422)
	if plain.Error() != "not enough grain (HTTP 422)" {
		t.Fatalf("unrelated error changed: %v", plain)
	}
}

func TestPassageResolvesShipNameAndPrintsIt(t *testing.T) {
	var posts []string
	llmServer(t, &posts)
	out, err := runLLM(t, passageCmd(), "--id", "m1", "--ship", "sea swallow")
	if err != nil {
		t.Fatal(err)
	}
	if len(posts) != 1 || !strings.Contains(out, "Sea Swallow sails") {
		t.Fatalf("posts=%v out=%s", posts, out)
	}
}
