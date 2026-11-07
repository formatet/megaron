package main

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// World blessings (steg 5a): fertility, the Builder's Blessing and the scapegoat name no unit; the
// list shows the steps and prices, the cast refuses an unoffered step before anything is spent,
// and the Builder's Blessing says which builds are ready sooner.
const riteWorldCult = `{"kharis":{},"temples":[],"unbuilt_temples":[],"prayers":[
 {"id":"minoan_potnia_fertility","name":"Fruitful Season","god":"Potnia","god_id":"potnia","settlement_id":"capital","city_name":"Knossos",
  "offering":{"livestock":4,"grain":4},"affordable":true,"can_prepare_offering":true,"scope_cost_factors":{"city":2,"realm":4},
  "targets":{"kind":"world","always":true,"unit":[],"city":{"name":"Knossos","count":1},"realm":{"count":3}}},
 {"id":"minoan_potnia_scapegoat","name":"The Scapegoat","god":"Potnia","god_id":"potnia","settlement_id":"capital","city_name":"Knossos",
  "offering":{"livestock":3},"affordable":true,"can_prepare_offering":true,"scope_cost_factors":{"realm":4},
  "targets":{"kind":"world","always":true,"unit":[],"city":{"name":"Knossos","count":1},"realm":{"count":3}}}]}`

func worldClient(t *testing.T, post string) {
	round3Client(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/cult"):
			fmt.Fprint(w, riteWorldCult)
		case strings.HasSuffix(r.URL.Path, "/provinces"):
			fmt.Fprint(w, `[{"id":"capital-prov","settlement_id":"capital","own":true,"is_capital":true}]`)
		case strings.HasSuffix(r.URL.Path, "/rite") && r.Method == http.MethodPost:
			fmt.Fprint(w, post)
		default:
			fmt.Fprint(w, `{"settlement":{}}`)
		}
	})
}

func TestRiteList_WorldBlessingShowsCityAndRealmPrices(t *testing.T) {
	worldClient(t, `{}`)
	c := riteCmd()
	c.SetArgs([]string{"--list"})
	out, err := captureStdout(t, c.Execute)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"--scope city  ×2", "grain×8 livestock×8", "--scope realm ×4", "all 3 of your cities", "livestock×12"} {
		if !strings.Contains(out, want) {
			t.Fatalf("--list lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "--scope unit") || strings.Contains(out, "--scope permanent") {
		t.Fatalf("a world blessing offered a unit step:\n%s", out)
	}
	// the scapegoat has no city step
	scape := out[strings.Index(out, "The Scapegoat"):]
	if strings.Contains(scape, "--scope city") {
		t.Fatalf("the scapegoat offered a city step:\n%s", scape)
	}
}

func TestRiteCast_WorldBlessingRefusesAStepItDoesNotOfferBeforeAnythingIsSpent(t *testing.T) {
	posted := false
	round3Client(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/cult"):
			fmt.Fprint(w, riteWorldCult)
		case strings.HasSuffix(r.URL.Path, "/provinces"):
			fmt.Fprint(w, `[{"id":"capital-prov","settlement_id":"capital","own":true,"is_capital":true}]`)
		case strings.HasSuffix(r.URL.Path, "/rite") && r.Method == http.MethodPost:
			posted = true
			fmt.Fprint(w, `{}`)
		default:
			fmt.Fprint(w, `{"settlement":{}}`)
		}
	})
	c := riteCmd()
	c.SetArgs([]string{"--prayer", "minoan_potnia_scapegoat", "--scope", "city", "--yes"})
	_, err := captureStdout(t, c.Execute)
	if err == nil || !strings.Contains(err.Error(), "choose realm") || posted {
		t.Fatalf("err=%v posted=%v: the scapegoat was said for a city", err, posted)
	}
}

func TestRiteCast_BuilderBlessingNamesWhatIsReadySooner(t *testing.T) {
	worldClient(t, `{"success":true,"mood":"Content","prayer":"minoan_velchanos_building","message":"ok","effect":{"sped_up":[{"kind":"build","id":"b1","old_due_tick":112,"new_due_tick":109},{"kind":"ship","id":"s1","old_due_tick":108,"new_due_tick":106}]}}`)
	c := riteCmd()
	c.SetArgs([]string{"--prayer", "minoan_potnia_fertility", "--scope", "realm", "--yes"})
	out, err := captureStdout(t, c.Execute)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Building b1 is ready 3 game days sooner.", "Ship under construction s1 is ready 2 game days sooner."} {
		if !strings.Contains(out, want) {
			t.Fatalf("lacks %q:\n%s", want, out)
		}
	}
}

func TestCityStatusLines_NameWhatStandsOverTheCityAndTheGoat(t *testing.T) {
	sett := map[string]any{"city_blessings": []any{
		map[string]any{"effect": "blessed_fertility", "prayer": "Fruitful Season", "scope": "city", "remaining_game_days": 12.0},
		map[string]any{"effect": "blessed_building", "prayer": "The Builder's Blessing", "scope": "realm", "remaining_game_days": 1.0},
	}}
	lines := cityBlessingLines(sett)
	if len(lines) != 2 || !strings.Contains(lines[0], "Fruitful Season on this city: people and herds grow half as fast again") || !strings.Contains(lines[0], "12 game days left") ||
		!strings.Contains(lines[1], "on the whole realm") || !strings.Contains(lines[1], "three quarters of the time") || !strings.Contains(lines[1], "1 game day left") {
		t.Fatalf("lines %q", lines)
	}
	if got := scapegoatLine(map[string]any{"scapegoat_remaining_game_days": 20.0}); !strings.Contains(got, "scapegoat stands for your realm for 20 game days") || !strings.Contains(got, "borne by it") {
		t.Fatalf("goat line %q", got)
	}
	if scapegoatLine(map[string]any{}) != "" {
		t.Fatal("a goat was named though none stands")
	}
	if !strings.Contains(buildQueueState(nil, map[string]any{"active": true, "complete_at": "x", "blessed": true}), "the Builder's Blessing") {
		t.Fatal("a cut build is not marked")
	}
}
