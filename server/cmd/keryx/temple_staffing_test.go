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
