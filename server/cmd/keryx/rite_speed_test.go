package main

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// Speed prayers (steg 2b): a Runner/caravan prayer lists the journeys under way and says that the
// city and realm steps hold for departures too; the cast reply names what was shortened.
const riteSpeedCult = `{"kharis":{},"temples":[],"unbuilt_temples":[],"prayers":[
 {"id":"minoan_asterion_swift","name":"Swift Feet","god":"Asterion","god_id":"asterion","settlement_id":"capital","city_name":"Knossos",
  "offering":{"timber":4,"silver":3},"affordable":true,"can_prepare_offering":true,"scope_cost_factors":{"unit":1,"city":2,"realm":4},
  "targets":{"always":true,"unit":[{"id":"33333333-3333-3333-3333-333333333333","name":"Runner to Phaistos","where":"arrives in 4 game days"}],
   "city":{"name":"Knossos","count":1},"realm":{"count":1}}}]}`

func TestRiteList_SpeedPrayerHoldsForDeparturesToo(t *testing.T) {
	round3Client(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/cult") {
			fmt.Fprint(w, riteSpeedCult)
			return
		}
		fmt.Fprint(w, `{"settlement":{}}`)
	})
	c := riteCmd()
	c.SetArgs([]string{"--list"})
	out, err := captureStdout(t, c.Execute)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Runner to Phaistos", "arrives in 4 game days", "every Runner and caravan of Knossos", "while the prayer lasts", "silver×6 timber×8"} {
		if !strings.Contains(out, want) {
			t.Fatalf("--list lacks %q:\n%s", want, out)
		}
	}
}

func TestRiteCast_SaysWhichJourneysArriveSooner(t *testing.T) {
	round3Client(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/cult"):
			fmt.Fprint(w, riteSpeedCult)
		case strings.HasSuffix(r.URL.Path, "/provinces"):
			fmt.Fprint(w, `[{"id":"capital-prov","settlement_id":"capital","own":true,"is_capital":true}]`)
		case strings.HasSuffix(r.URL.Path, "/rite") && r.Method == http.MethodPost:
			fmt.Fprint(w, `{"success":true,"mood":"Content","prayer":"minoan_asterion_swift","message":"ok","effect":{"sped_up":[{"kind":"messenger","id":"33333333-3333-3333-3333-333333333333","old_arrive_tick":108,"new_arrive_tick":106}]}}`)
		default:
			fmt.Fprint(w, `{"settlement":{}}`)
		}
	})
	c := riteCmd()
	c.SetArgs([]string{"--prayer", "minoan_asterion_swift", "--scope", "realm", "--yes"})
	out, err := captureStdout(t, c.Execute)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Runner 33333333-3333-3333-3333-333333333333 arrives 2 game days sooner.") {
		t.Fatalf("no sooner-line:\n%s", out)
	}
}
