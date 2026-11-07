package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"testing"
)

var regexpTick = regexp.MustCompile(`(?i)\bticks?\b`)

const riteWallsCult = `{"kharis":{},"temples":[],"unbuilt_temples":[],"prayers":[
 {"id":"minoan_potnia_walls_fall","name":"The Walls Fall","god":"Potnia","god_id":"potnia","settlement_id":"capital","city_name":"Knossos",
  "offering":{"livestock":12,"silver":8},"affordable":true,"can_prepare_offering":true,
  "targets":{"unit":[],"city":{"count":0},"realm":{"count":0},"kind":"city","cities":[{"id":"bbbbbbbb-0000-0000-0000-000000000001","name":"Mycenae","where":"held by Agamemnon, near (20,20), wall level 2 as you last saw it"}]}}]}`

func riteWallsServer(t *testing.T, posted *map[string]any) {
	round3Client(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/cult"):
			fmt.Fprint(w, riteWallsCult)
		case strings.HasSuffix(r.URL.Path, "/provinces"):
			fmt.Fprint(w, `[{"id":"capital-prov","settlement_id":"capital","own":true,"is_capital":true}]`)
		case strings.HasSuffix(r.URL.Path, "/rite") && r.Method == http.MethodPost:
			raw, _ := io.ReadAll(r.Body)
			*posted = map[string]any{}
			_ = json.Unmarshal(raw, posted)
			fmt.Fprint(w, `{"success":true,"mood":"Content","prayer":"x","message":"ok"}`)
		default:
			fmt.Fprint(w, `{"settlement":{}}`)
		}
	})
}

func TestRiteList_ShowsTheCitiesWhoseWallsPotniaCanBringDown(t *testing.T) {
	var posted map[string]any
	riteWallsServer(t, &posted)
	c := riteCmd()
	c.SetArgs([]string{"--list"})
	out, err := captureStdout(t, c.Execute)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"--target <name>", "Mycenae", "held by Agamemnon", "bbbbbbbb-0000-0000-0000-000000000001"} {
		if !strings.Contains(out, want) {
			t.Fatalf("--list lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "×0") || regexpTick.MatchString(out) {
		t.Fatalf("a wall prayer must not print a price per scope step or internal time:\n%s", out)
	}
}

func TestRiteCast_WallsFallByCityNameAsGiven(t *testing.T) {
	var posted map[string]any
	riteWallsServer(t, &posted)
	c := riteCmd()
	c.SetArgs([]string{"--prayer", "minoan_potnia_walls_fall", "--target", "mycenae", "--yes"})
	if _, err := captureStdout(t, c.Execute); err != nil {
		t.Fatal(err)
	}
	if posted["target"] != "bbbbbbbb-0000-0000-0000-000000000001" || posted["target_scope"] != nil {
		t.Fatalf("posted %v", posted)
	}
	c = riteCmd()
	c.SetArgs([]string{"--prayer", "minoan_potnia_walls_fall", "--target", "Nowhere", "--yes"})
	if _, err := captureStdout(t, c.Execute); err == nil || !strings.Contains(err.Error(), "Mycenae") {
		t.Fatalf("an unknown city must list the ones on the map: %v", err)
	}
	c = riteCmd()
	c.SetArgs([]string{"--prayer", "minoan_potnia_walls_fall", "--target", "Mycenae", "--scope", "realm", "--yes"})
	if _, err := captureStdout(t, c.Execute); err == nil {
		t.Fatal("--scope has no meaning for the wall prayer")
	}
}

func TestWallsNotices_AreReadableAndNeverNameTheCaster(t *testing.T) {
	omen := wallsNoticeText("WallsOmen", "Tiryns", 0)
	fell := wallsNoticeText("WallsFell", "Tiryns", 1)
	gone := wallsNoticeText("WallsFell", "Tiryns", 0)
	for _, s := range []string{omen, fell, gone} {
		if !strings.Contains(s, "Tiryns") || regexpTick.MatchString(s) {
			t.Fatalf("notice %q", s)
		}
	}
	if !strings.Contains(omen, "someone has prayed") || !strings.Contains(fell, "level 1") || !strings.Contains(gone, "gone") {
		t.Fatalf("%q / %q / %q", omen, fell, gone)
	}
}
