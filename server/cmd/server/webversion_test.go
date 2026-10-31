package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWebVersionChangesWithClientCodeOnly(t *testing.T) {
	dir := t.TempDir()
	js := filepath.Join(dir, "js", "a.js")
	png := filepath.Join(dir, "img.png")
	if err := os.MkdirAll(filepath.Dir(js), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{js, png} {
		if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	v := newWebVersion(dir, "abc1234")
	v.every = 0
	first := v.current()

	if err := os.WriteFile(png, []byte("bigger image"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := v.current(); got != first {
		t.Fatalf("an image is not client code: %q -> %q", first, got)
	}

	later := time.Now().Add(time.Minute)
	if err := os.Chtimes(js, later, later); err != nil {
		t.Fatal(err)
	}
	if got := v.current(); got == first {
		t.Fatalf("a deployed module must change the version, still %q", got)
	}
	if other := newWebVersion(dir, "def5678"); other.current() == v.current() {
		t.Fatal("a new server build must change the version")
	}
}

func TestWebVersionHeaderOnAPIOnly(t *testing.T) {
	v := newWebVersion(t.TempDir(), "abc1234")
	h := v.middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	for path, want := range map[string]bool{"/api/v1/worlds/x/map": true, "/static/js/a.js": false} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if got := rec.Header().Get(versionHeader) != ""; got != want {
			t.Fatalf("%s: header present=%v, want %v", path, got, want)
		}
	}
}
