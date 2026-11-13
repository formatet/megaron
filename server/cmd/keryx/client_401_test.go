package main

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

// An expired session must tell a CLI player how to get back in — the server's
// own 401 text says "reload the page", which is the web's way, not keryx's.
func TestAPIError_Unauthorized_PointsAtKeryxLogin(t *testing.T) {
	body := []byte(`{"error":"Your session has expired — reload the page to log in again."}`)
	msg := apiError(body, 401).Error()
	if !strings.Contains(msg, "keryx login") {
		t.Errorf("401 error %q does not name keryx login", msg)
	}
	if strings.Contains(msg, "reload the page") {
		t.Errorf("401 error %q carries the web's instruction into the terminal", msg)
	}
	// Other statuses still pass the server's own words through.
	if got := apiError([]byte(`{"error":"not your province"}`), 403).Error(); got != "not your province (HTTP 403)" {
		t.Errorf("403 passthrough = %q", got)
	}
}

// An expired access token is renewed once with the saved refresh token and the
// request repeated; a refresh that fails leaves today's 401 text.
func TestClient401RefreshesOnceAndRetries(t *testing.T) {
	t.Setenv("POLEIA_CONFIG", filepath.Join(t.TempDir(), "config.json"))
	t.Setenv("POLEIA_TOKEN", "")
	refreshes, gets := 0, 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/auth/refresh":
			refreshes++
			if refreshes > 1 || !strings.Contains(readBody(r), "rt-old") {
				w.WriteHeader(401)
				return
			}
			fmt.Fprint(w, `{"access_token":"at-new","refresh_token":"rt-new"}`)
		default:
			gets++
			if r.Header.Get("Authorization") != "Bearer at-new" {
				w.WriteHeader(401)
				fmt.Fprint(w, `{"error":"expired"}`)
				return
			}
			fmt.Fprint(w, `{"ok":true}`)
		}
	}))
	defer ts.Close()
	cfg := &Config{Server: ts.URL, Token: "at-old", RefreshToken: "rt-old"}
	c := newClient(cfg)
	if _, err := c.get("/api/v1/x"); err != nil {
		t.Fatalf("expected a transparent refresh, got %v", err)
	}
	if gets != 2 || refreshes != 1 || cfg.Token != "at-new" || cfg.RefreshToken != "rt-new" {
		t.Fatalf("gets=%d refreshes=%d cfg=%+v", gets, refreshes, cfg)
	}
	// Second client: refresh token is now spent on the server side -> today's text.
	cfg2 := &Config{Server: ts.URL, Token: "at-old", RefreshToken: "rt-old"}
	if _, err := newClient(cfg2).get("/api/v1/x"); err == nil || !strings.Contains(err.Error(), "keryx login") {
		t.Fatalf("failed refresh must give the login hint, got %v", err)
	}
}

func readBody(r *http.Request) string {
	b, _ := io.ReadAll(r.Body)
	return string(b)
}

func TestRefusalHintAddsTheTerminalCommand(t *testing.T) {
	cases := map[string]string{
		"no passable route to (1,61) for this unit — a sea crossing needs a ship — load the unit aboard a ship in the same city first (both garrisoned)": "keryx unit load --ship <id> --unit <id>",
		"none of your men have ever seen (1,2) — only an explorer goes into unknown land: march there with intent \"explore\"":                           "--intent explore",
		"no citizens left in the pool — every citizen is already placed; take one off another place first":                                               "keryx place <good> -1",
	}
	for msg, want := range cases {
		if got := refusalHint(errors.New(msg)).Error(); !strings.Contains(got, want) {
			t.Errorf("%q: hint %q missing from %q", msg, want, got)
		}
	}
	if got := refusalHint(errors.New("nothing to add")).Error(); got != "nothing to add" {
		t.Errorf("unrelated error changed: %q", got)
	}
}
