package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// keryx must list and resolve the same Codex articles the web shows.
func TestCodexIndexListsEveryArticle(t *testing.T) {
	data, err := os.ReadFile("../../../web/static/codex/index.json")
	if err != nil {
		t.Fatal(err)
	}
	var idx codexIndex
	if err := json.Unmarshal(data, &idx); err != nil {
		t.Fatal(err)
	}
	out := formatCodexIndex(idx)
	for _, a := range idx.Articles {
		if !strings.Contains(out, a.ID) {
			t.Errorf("article %q missing from keryx codex list", a.ID)
		}
		if _, err := os.Stat("../../../web/static/codex/" + a.ID + ".md"); err != nil {
			t.Errorf("article %q has no file: %v", a.ID, err)
		}
	}
	if _, _, ok := codexArticle(idx, "bronze"); !ok {
		t.Errorf("bronze not resolvable")
	}
	if _, _, ok := codexArticle(idx, "../../etc/passwd"); ok {
		t.Errorf("a path resolved as an article id")
	}
}
