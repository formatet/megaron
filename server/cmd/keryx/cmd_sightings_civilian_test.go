package main

import (
	"strings"
	"testing"
)

func TestForeignCivilianSightingsCurrentHeadingAndNoOwnMovers(t *testing.T) {
	rows, err := foreignCivilianSightings([]byte(`[{"sender":"Nestor","own":false,"current_q":0,"current_r":0,"heading":"north-east"},{"owner":"Eirene","mine":true,"current_q":2,"current_r":3},{"sender":"Eirene","own":true,"current_q":2,"current_r":3},{"sender":"Hidden"}]`))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Heading != "north-east" || *rows[0].CurrentQ != 0 {
		t.Fatalf("foreign civilian sightings lost current heading or included own/unknown actor: %+v", rows)
	}
	text := capturePrint(t, func() { printCivilianSighting(rows[0], "Runner") })
	if !strings.Contains(text, "Nestor's Runner at (0,0), heading north-east") {
		t.Fatalf("civilian sighting lacks readable heading: %q", text)
	}
}
