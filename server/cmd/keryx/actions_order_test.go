package main

import (
	"sort"
	"testing"
)

// The category overview lists categories alphabetically (Timothy 2026-11-01).
func TestCategoryOrderAlphabetical(t *testing.T) {
	if !sort.StringsAreSorted(categoryOrder) {
		t.Fatalf("categoryOrder must be alphabetical, got %v", categoryOrder)
	}
	if len(categoryOrder) != 6 {
		t.Fatalf("expected six categories, got %v", categoryOrder)
	}
}
