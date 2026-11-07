package main

import (
	"strings"
	"testing"
)

// Böner steg 4: what has been laid on a unit for ever shows in the owner's lists, and a ship with Stout Hulls reads x/6.
func TestFormatSize_ShowsTheHullCeilingAndTheBlessingsForEver(t *testing.T) {
	ship := unitRow{Category: "naval", Status: "marching", Crew: 20, Hull: 6, HullMax: 6, ProvisionDays: 9,
		Blessings: []string{"Stout Hulls for ever", "followed by the Sea Daimons, for ever"}}
	got := formatSize(nil, ship)
	for _, want := range []string{"hull 6/6", "Stout Hulls for ever", "followed by the Sea Daimons, for ever"} {
		if !strings.Contains(got, want) {
			t.Fatalf("ship line lacks %q: %s", want, got)
		}
	}
	ship.Hull = 4
	if got := formatSize(nil, ship); !strings.Contains(got, "hull 4/6 damaged") {
		t.Fatalf("a damaged blessed ship should read 4/6: %s", got)
	}
	plain := unitRow{Category: "naval", Status: "marching", Crew: 20, Hull: 5, ProvisionDays: 9}
	if got := formatSize(nil, plain); strings.Contains(got, "hull") || strings.Contains(got, "for ever") {
		t.Fatalf("an ordinary full ship says nothing about hull: %s", got)
	}
	troop := unitRow{Category: "land", Status: "garrison", Size: 40, Blessings: []string{"blessed by Velchanos, for ever"}}
	if got := formatSize(nil, troop); got != "40 men — blessed by Velchanos, for ever" {
		t.Fatalf("troop line: %q", got)
	}
}
