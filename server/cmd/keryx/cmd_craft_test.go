package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A recipe name that is NOT in recipeNameToID must still resolve if the server
// publishes it — this is the whole point of the catalogue fallback. Without it,
// a recipe added by a migration is unreachable by name until someone edits the
// hardcoded map, which is the same staleness bug the web client had until
// /api/v1/recipes landed.
func TestResolveRecipeID_FallsBackToServerCatalogue(t *testing.T) {
	var hits int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/recipes" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		hits++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"id":1,"output_key":"bronze","ingredients":[{"good_key":"copper","quantity":9}]},
			{"id":2,"output_key":"luxury","ingredients":[]},
			{"id":7,"output_key":"faience","ingredients":[]}
		]`))
	}))
	defer ts.Close()
	c := newClient(&Config{Server: ts.URL})

	// Unknown to the offline map, known to the server.
	id, err := resolveRecipeID(c, "faience")
	if err != nil {
		t.Fatalf("faience: unexpected error: %v", err)
	}
	if id != 7 {
		t.Errorf("faience: got id %d, want 7", id)
	}
	// Case-insensitive, like the offline path.
	if id, err = resolveRecipeID(c, "  FAIENCE  "); err != nil || id != 7 {
		t.Errorf("padded/uppercase: got (%d, %v), want (7, nil)", id, err)
	}
	if hits != 2 {
		t.Errorf("expected 2 catalogue fetches, got %d", hits)
	}
}

// The offline map is a fast path, not just a cache: a name it knows must never
// cost a request. Guarded because losing this silently would add a round-trip
// to every craft the bot fleet issues.
func TestResolveRecipeID_KnownNameSkipsTheServer(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("server was called for a name in recipeNameToID (%s)", r.URL.Path)
	}))
	defer ts.Close()
	c := newClient(&Config{Server: ts.URL})

	for name, want := range map[string]int{"bronze": 1, "LUXURY": 2, " bronze ": 1} {
		got, err := resolveRecipeID(c, name)
		if err != nil {
			t.Errorf("%q: unexpected error: %v", name, err)
			continue
		}
		if got != want {
			t.Errorf("%q: got id %d, want %d", name, got, want)
		}
	}
}

// A name neither side knows must name what the SERVER has, so the reader can
// discover a recipe this binary predates.
func TestResolveRecipeID_UnknownNameListsServerRecipes(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":7,"output_key":"faience"},{"id":1,"output_key":"bronze"}]`))
	}))
	defer ts.Close()
	c := newClient(&Config{Server: ts.URL})

	_, err := resolveRecipeID(c, "obsidian")
	if err == nil {
		t.Fatal("expected an error for an unknown recipe")
	}
	msg := err.Error()
	for _, want := range []string{`"obsidian"`, "bronze", "faience"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q missing %q", msg, want)
		}
	}
	// Sorted, so the same mistake prints the same message every run.
	if i, j := strings.Index(msg, "bronze"), strings.Index(msg, "faience"); i > j {
		t.Errorf("recipe names not sorted in %q", msg)
	}
}

// Unreachable server: still a useful error naming the recipes we know offline,
// and saying why the list may be incomplete — never a bare failure.
func TestResolveRecipeID_ServerDownStillNamesKnownRecipes(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer ts.Close()
	c := newClient(&Config{Server: ts.URL})

	_, err := resolveRecipeID(c, "faience")
	if err == nil {
		t.Fatal("expected an error when the catalogue is unreachable")
	}
	msg := err.Error()
	for _, want := range []string{`"faience"`, "bronze", "luxury", "could not reach the server"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q missing %q", msg, want)
		}
	}
}

func TestKnownRecipeNames_Sorted(t *testing.T) {
	got := knownRecipeNames()
	want := []string{"bronze", "luxury"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}
