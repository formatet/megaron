package main

import (
	"encoding/json"
	"testing"
)

func TestCodexSearchTitleAliasBodyOrderAndSafeResolution(t *testing.T) {
	var idx codexIndex
	err := json.Unmarshal([]byte(`{"articles":[{"id":"sea","title":"Sea","aliases":["ship"]},{"id":"galley","title":"Galley","aliases":["ship"]},{"id":"timber","title":"Timber"},{"id":"kharis","title":"Kharis","aliases":["mana"]},{"id":"elite-infantry","title":"Bronze Guard"}]}`), &idx)
	if err != nil {
		t.Fatal(err)
	}
	all := searchCodexArticles("galley", idx, map[string]string{"sea": "Galley carrier", "timber": "A galley needs wood"})
	if len(all) != 3 || all[0].ID != "galley" || all[1].ID != "sea" || all[2].ID != "timber" {
		t.Fatalf("title/body ordering: %+v", all)
	}
	if _, ok := resolveCodexArticle(idx, "ship"); ok {
		t.Fatal("ambiguous alias picked an arbitrary article")
	}
	for query, want := range map[string]string{"galley": "galley", "SHIP": "galley", "mana": "kharis", "bronze_guard": "elite-infantry"} {
		hits := searchCodexArticles(query, idx, map[string]string{"sea": "Galley carrier", "timber": "A galley needs wood"})
		if len(hits) == 0 || hits[0].ID != want {
			t.Errorf("query %q first=%v want%s", query, hits, want)
		}
	}
	for input, want := range map[string]string{"Bronze Guard": "elite-infantry", "mana": "kharis", "galley": "galley"} {
		a, ok := resolveCodexArticle(idx, input)
		if !ok || a.ID != want {
			t.Errorf("resolve %q=%+v %v", input, a, ok)
		}
	}
	if _, ok := resolveCodexArticle(idx, "../../etc/passwd"); ok {
		t.Fatal("path resolved")
	}
}

// An old fragment id is a section now: keryx resolves it to the section and
// prints only that part of its article.
func TestCodexSectionIDResolvesAndPrintsOnlyItsSection(t *testing.T) {
	var idx codexIndex
	if err := json.Unmarshal([]byte(`{"articles":[{"id":"colonies","title":"Colonies","sections":[{"id":"colonies-abandoning-a-colony","title":"Abandoning a colony","aliases":["abandon"]}]}]}`), &idx); err != nil {
		t.Fatal(err)
	}
	for _, input := range []string{"colonies-abandoning-a-colony", "Abandoning a colony", "abandon"} {
		a, ok := resolveCodexArticle(idx, input)
		if !ok || a.ID != "colonies-abandoning-a-colony" || a.Parent != "colonies" {
			t.Errorf("resolve %q = %+v %v", input, a, ok)
		}
	}
	body := "Intro.\n\n## Founding {#f}\n\nFound.\n\n## Abandoning a colony {#colonies-abandoning-a-colony}\n\nLeave it.\n\n### Detail\n\nMore.\n\n## Next {#n}\n\nOther."
	if got := codexSectionText(body, "colonies-abandoning-a-colony"); got != "Leave it.\n\n### Detail\n\nMore." {
		t.Errorf("section text %q", got)
	}
	if got := codexSectionText(body, ""); got != "Intro." {
		t.Errorf("intro %q", got)
	}
	hits := searchCodexArticles("leave it", idx, map[string]string{"colonies": body})
	if len(hits) != 1 || hits[0].ID != "colonies-abandoning-a-colony" {
		t.Errorf("body hit should name the section: %+v", hits)
	}
}
