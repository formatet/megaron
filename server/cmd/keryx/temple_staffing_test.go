package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestStaffTemplePostsPhysicalPlacement(t *testing.T) {
	posts := 0
	round3Client(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body["target_kind"] != "building" || body["building_type"] != "temple" || body["good_key"] != "cult" {
				t.Fatalf("placement %v", body)
			}
			posts++
			w.WriteHeader(201)
			fmt.Fprint(w, `{"ordinal":1}`)
			return
		}
		fmt.Fprint(w, `{"total_gubbar":10,"pool_size":10,"hexes":[],"buildings":[{"building_type":"temple","level":1,"goods":[{"good_key":"cult","staffing_only":true,"cap":2,"placed":0,"placed_ordinals":[]}]}]}`)
	})
	cmd := staffCmd()
	cmd.SetArgs([]string{"temple", "+1"})
	_, err := captureStdout(t, cmd.Execute)
	if err != nil || posts != 1 {
		t.Fatalf("posts%d err%v", posts, err)
	}
	cap := 2
	out := goodCell(placementGood{GoodKey: "cult", StaffingOnly: true, Cap: &cap, Placed: 1})
	if !strings.Contains(out, "1/2 temple workers") || strings.Contains(out, "per game day") {
		t.Fatal(out)
	}
}

// The help example puts --god after the delta (`staff temple +1 --god potnia`);
// flags after a signed delta must still be read (AI report 2026-11-01).
func TestStaffTempleGodFlagAfterDelta(t *testing.T) {
	posts := 0
	round3Client(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body["god"] != "potnia" {
				t.Fatalf("placement %v", body)
			}
			posts++
			w.WriteHeader(201)
			fmt.Fprint(w, `{"ordinal":1}`)
			return
		}
		fmt.Fprint(w, `{"total_gubbar":10,"pool_size":10,"hexes":[],"buildings":[`+
			`{"building_type":"temple","god_id":"velchanos","level":1,"goods":[{"good_key":"cult","staffing_only":true,"cap":2,"placed":0,"placed_ordinals":[]}]},`+
			`{"building_type":"temple","god_id":"potnia","level":1,"goods":[{"good_key":"cult","staffing_only":true,"cap":2,"placed":0,"placed_ordinals":[]}]}]}`)
	})
	for _, args := range [][]string{{"temple", "+2", "--god", "potnia"}, {"temple", "+2", "--god=potnia"}, {"--god", "potnia", "temple", "+2"}} {
		posts = 0
		cmd := staffCmd()
		cmd.SetArgs(args)
		if _, err := captureStdout(t, cmd.Execute); err != nil || posts != 2 {
			t.Fatalf("%v: posts%d err%v", args, posts, err)
		}
	}
}

// staff --god X must read back temple X, not the first temple in the list.
func TestStaffGodReadbackMatchesGod(t *testing.T) {
	round3Client(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.WriteHeader(201)
			fmt.Fprint(w, `{"ordinal":1}`)
			return
		}
		fmt.Fprint(w, `{"total_gubbar":10,"pool_size":8,"hexes":[],"buildings":[`+
			`{"building_type":"temple","god_id":"potnia","level":1,"goods":[{"good_key":"cult","staffing_only":true,"cap":2,"placed":2,"placed_ordinals":[1,2]}]},`+
			`{"building_type":"temple","god_id":"velchanos","level":1,"goods":[{"good_key":"cult","staffing_only":true,"cap":2,"placed":0,"placed_ordinals":[]}]}]}`)
	})
	cmd := staffCmd()
	cmd.SetArgs([]string{"--god", "potnia", "temple", "+1"})
	out, err := captureStdout(t, cmd.Execute)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "2/2 temple workers (potnia)") || strings.Contains(out, "0/2") || !strings.Contains(out, "on temple (potnia)") {
		t.Fatal(out)
	}
}
