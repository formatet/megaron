package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestStaffShipyardPostsPhysicalPlacement(t *testing.T) {
	posts := 0
	round3Client(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body["target_kind"] != "building" || body["building_type"] != "shipyard" || body["good_key"] != "shipbuilding" {
				t.Fatalf("placement %v", body)
			}
			posts++
			w.WriteHeader(201)
			fmt.Fprint(w, `{"ordinal":1}`)
			return
		}
		fmt.Fprint(w, `{"total_gubbar":10,"pool_size":10,"hexes":[],"buildings":[{"building_type":"shipyard","level":1,"goods":[{"good_key":"shipbuilding","staffing_only":true,"cap":3,"placed":0,"placed_ordinals":[]}]}]}`)
	})
	cmd := staffCmd()
	cmd.SetArgs([]string{"shipyard", "+1"})
	_, err := captureStdout(t, cmd.Execute)
	if err != nil || posts != 1 {
		t.Fatalf("posts%d err%v", posts, err)
	}
}

func TestShipyardWaitingSuppressesETA(t *testing.T) {
	for _, status := range []string{"forming", "repairing"} {
		out := formatSize(nil, unitRow{Category: "naval", Status: status, ShipyardWaiting: true})
		if out != "waiting for shipyard workers" {
			t.Fatal(out)
		}
	}
	cap := 3
	for _, placed := range []int{0, 1, 3} {
		out := goodCell(placementGood{GoodKey: "shipbuilding", StaffingOnly: true, Cap: &cap, Placed: placed})
		want := fmt.Sprintf("%dx working pace", placed)
		if placed == 0 {
			want = "waiting for shipyard workers"
		}
		if !strings.Contains(out, want) || strings.Contains(out, "temple") || strings.Contains(out, "per game day") {
			t.Fatal(out)
		}
	}
}

func TestPaidShipyardOrdersPrintWaiting(t *testing.T) {
	round3Client(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"names":["Asterion"],"shipyard_waiting":true,"complete_at":null,"hull_before":2,"hull_target":5,"good":"timber","amount":3,"duration_ticks":3}`)
	})
	recruit := recruitCmd()
	recruit.SetArgs([]string{"--unit", "galley"})
	out, err := captureStdout(t, recruit.Execute)
	if err != nil || !strings.Contains(strings.ToLower(out), "waiting for shipyard workers") || strings.Contains(out, "Ready ") {
		t.Fatalf("%s %v", out, err)
	}
	repair := unitRepairCmd()
	repair.SetArgs([]string{"--unit", "12345678-0000-0000-0000-000000000000"})
	out, err = captureStdout(t, repair.Execute)
	if err != nil || !strings.Contains(out, "waiting for shipyard workers") || strings.Contains(out, "ready ") {
		t.Fatalf("%s %v", out, err)
	}
}

func TestWorkshopBuildersAndQueueStateAreTold(t *testing.T) {
	cap := 3
	if out := goodCell(placementGood{GoodKey: "building", StaffingOnly: true, Cap: &cap, Placed: 0}); !strings.Contains(out, "base building pace 1x") {
		t.Fatal(out)
	}
	if out := goodCell(placementGood{GoodKey: "building", StaffingOnly: true, Cap: &cap, Placed: 2}); !strings.Contains(out, "2/3 builders") || !strings.Contains(out, "2x building pace") {
		t.Fatal(out)
	}
	if out := buildQueueState(nil, map[string]any{"active": false, "work_left_game_days": 8.0}); !strings.Contains(out, "next in line") || !strings.Contains(out, "8 game days of work") {
		t.Fatal(out)
	}
	if out := buildQueueState(nil, map[string]any{"active": true, "rate": 3.0, "complete_at": "2099-01-01T00:00:00Z"}); !strings.Contains(out, "3x builders' pace") {
		t.Fatal(out)
	}
}
