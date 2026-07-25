package main

import "testing"

// DB audit 2026-07-25: 38% of the world's labor sat on goods already at the
// storage cap, discarding everything produced above it — and no client, CLI
// or web, said so. atStorageCeiling/capFootnote are the pure pieces that
// surface it in `keryx goods`; printCurrentAllocation (cmd_allocate.go)
// reuses atStorageCeiling for the same marker in `keryx allocate`.

func TestAtStorageCeiling(t *testing.T) {
	cases := []struct {
		name   string
		amount float64
		cap    float64
		want   bool
	}{
		{"at cap", 1000, 1000, true},
		{"just under threshold", 980, 1000, false},
		{"within threshold band", 991, 1000, true},
		{"over cap via a delivery that lands late", 1010, 1000, true},
		{"cap is zero (unbounded good)", 500, 0, false},
		{"zero stock, zero cap", 0, 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := atStorageCeiling(c.amount, c.cap); got != c.want {
				t.Errorf("atStorageCeiling(%v, %v) = %v, want %v", c.amount, c.cap, got, c.want)
			}
		})
	}
}

func TestCapFootnote_Empty(t *testing.T) {
	if got := capFootnote(nil); got != "" {
		t.Errorf("expected empty string for no notes, got %q", got)
	}
}

func TestCapFootnote_NamesLaborShareOnlyWhenAllocated(t *testing.T) {
	got := capFootnote([]capNote{
		{key: "grain", percent: 100},
		{key: "timber", percent: 30},
		{key: "stone", percent: 0}, // at cap but nobody is working it
	})
	if !contains(got, "grain (100% of your labor)") {
		t.Errorf("expected grain's labor share called out, got %q", got)
	}
	if !contains(got, "timber (30% of your labor)") {
		t.Errorf("expected timber's labor share called out, got %q", got)
	}
	if !contains(got, "stone") {
		t.Errorf("expected unallocated stone still named, got %q", got)
	}
	if contains(got, "stone (") {
		t.Errorf("stone has no labor on it — must not get a percent parenthetical, got %q", got)
	}
	if !contains(got, "discarded") || !contains(got, "Move that labor") {
		t.Errorf("footnote must explain the mechanism and the fix, got %q", got)
	}
}
