package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestOrdersEnRouteLine(t *testing.T) {
	c := &Client{tickSeconds: 3600, tickSecondsFetched: true}
	q, r := 47, 23
	arrives := time.Now().Add(2*time.Hour + 30*time.Minute).UTC().Format(time.RFC3339)
	got := ordersEnRouteLine(c, orderEnRoute{Verb: "march", TargetQ: &q, TargetR: &r, ArrivesAt: arrives})
	want := "Order on its way: march to (47,23) — the runner reaches it " + arrivalETA(c, arrives)
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if got := ordersEnRouteLine(c, orderEnRoute{Verb: "recall", ArrivesAt: arrives}); !strings.HasPrefix(got, "Order on its way: recall home — ") {
		t.Errorf("recall line = %q", got)
	}
	// No cadence: still names the order, never a wrong ETA.
	if got := ordersEnRouteLine(&Client{tickSecondsFetched: true}, orderEnRoute{Verb: "stance", ArrivesAt: arrives}); got != "Order on its way: stance change — the runner reaches it "+arrivalETA(c, arrives) {
		t.Errorf("no-cadence line = %q", got)
	}
}

func TestOrdersEnRoutePursuitUsesNextMeeting(t *testing.T) {
	c := &Client{tickSeconds: 3600, tickSecondsFetched: true}
	for _, legs := range []int{0, 2} {
		raw := fmt.Sprintf(`{"verb":"redirect","target_q":47,"target_r":23,"following_unit":true,"pursuit_legs":%d,"arrives_at":"2030-01-01T12:30:00Z"}`, legs)
		var o orderEnRoute
		if err := json.Unmarshal([]byte(raw), &o); err != nil {
			t.Fatal(err)
		}
		got := ordersEnRouteLine(c, o)
		for _, want := range []string{"redirect to (47,23)", "Runner following the unit", "next meeting"} {
			if !strings.Contains(got, want) {
				t.Errorf("pursuit line%q lacks%q", got, want)
			}
		}
		if strings.Contains(got, "the runner reaches it") {
			t.Errorf("pursuit promised delivery%q", got)
		}
		if legs > 0 && !strings.Contains(got, "new route") {
			t.Errorf("reroute missing%q", got)
		}
	}
	var noTime orderEnRoute
	if err := json.Unmarshal([]byte(`{"verb":"stance","following_unit":true,"arrives_at":""}`), &noTime); err != nil {
		t.Fatal(err)
	}
	got := ordersEnRouteLine(c, noTime)
	if !strings.Contains(got, "meeting time unavailable") {
		t.Errorf("unknown meeting%q", got)
	}
}

func TestOrdersEnRoutePursuitWaitHasNoDeliveryETA(t *testing.T) {
	var o orderEnRoute
	if err := json.Unmarshal([]byte(`{"verb":"recall","following_unit":true,"following_wait":true,"pursuit_legs":2,"arrives_at":"2030-01-01T12:30:00Z"}`), &o); err != nil {
		t.Fatal(err)
	}
	got := ordersEnRouteLine(&Client{}, o)
	if !strings.Contains(got, "Runner waiting to continue following the unit") {
		t.Errorf("waiting state absent%q", got)
	}
	for _, untrue := range []string{"next meeting", "2030", "runner reaches"} {
		if strings.Contains(got, untrue) {
			t.Errorf("retry time presented as meeting%q", got)
		}
	}
}
