package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

// TestSelfHealStaleProvince verifies that a 403 "not your province" triggers a
// one-shot capital re-resolve, persists the new province_id, and retries against it.
func TestSelfHealStaleProvince(t *testing.T) {
	const oldID = "11111111-1111-1111-1111-111111111111"
	const newID = "22222222-2222-2222-2222-222222222222"

	var resolveCalls, healedBuilds int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/provinces"):
			resolveCalls++
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[{"id":"` + newID + `","is_capital":false},` +
				`{"id":"` + newID + `","is_capital":true}]`))
		case strings.Contains(r.URL.Path, "/provinces/"+oldID+"/"):
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":"not your province"}`))
		case strings.Contains(r.URL.Path, "/provinces/"+newID+"/"):
			healedBuilds++
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	// Point config persistence at a temp file so saveConfig succeeds in the test.
	t.Setenv("POLEIA_CONFIG", filepath.Join(t.TempDir(), "config.json"))

	cfg := &Config{Server: ts.URL, WorldID: "world-1", ProvinceID: oldID}
	c := newClient(cfg)

	_, err := c.post("/api/v1/worlds/world-1/provinces/"+oldID+"/build", map[string]any{"type": "farm"})
	if err != nil {
		t.Fatalf("expected self-heal to succeed, got error: %v", err)
	}
	if cfg.ProvinceID != newID {
		t.Errorf("province_id not healed: got %q want %q", cfg.ProvinceID, newID)
	}
	if resolveCalls != 1 {
		t.Errorf("expected exactly 1 capital re-resolve, got %d", resolveCalls)
	}
	if healedBuilds != 1 {
		t.Errorf("expected retried build against new province, got %d", healedBuilds)
	}
}

// TestNoHealWhenCapitalUnchanged ensures we don't loop retrying when the 403 is a
// genuine ownership rejection (the resolved capital equals the stale ID).
func TestNoHealWhenCapitalUnchanged(t *testing.T) {
	const id = "11111111-1111-1111-1111-111111111111"

	var forbidden int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/provinces") {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[{"id":"` + id + `","is_capital":true}]`))
			return
		}
		forbidden++
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"not your province"}`))
	}))
	defer ts.Close()

	t.Setenv("POLEIA_CONFIG", filepath.Join(t.TempDir(), "config.json"))

	cfg := &Config{Server: ts.URL, WorldID: "world-1", ProvinceID: id}
	c := newClient(cfg)

	if _, err := c.post("/api/v1/worlds/world-1/provinces/"+id+"/build", map[string]any{}); err == nil {
		t.Fatal("expected error for genuine ownership rejection")
	}
	if forbidden != 1 {
		t.Errorf("expected the request to be attempted once (no retry loop), got %d", forbidden)
	}
}
