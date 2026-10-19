package main

import (
	"strings"
	"testing"
	"time"
)

func TestOrdersEnRouteLine(t *testing.T) {
	c := &Client{tickSeconds: 3600, tickSecondsFetched: true}
	q, r := 47, 23
	arrives := time.Now().Add(2*time.Hour + 30*time.Minute).UTC().Format(time.RFC3339)
	got := ordersEnRouteLine(c, orderEnRoute{Verb: "march", TargetQ: &q, TargetR: &r, ArrivesAt: arrives})
	want := "Order on its way: march to (47,23) — the runner reaches it in 3 days"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if got := ordersEnRouteLine(c, orderEnRoute{Verb: "recall", ArrivesAt: arrives}); !strings.HasPrefix(got, "Order on its way: recall home — ") {
		t.Errorf("recall line = %q", got)
	}
	// No cadence: still names the order, never a wrong ETA.
	if got := ordersEnRouteLine(&Client{tickSecondsFetched: true}, orderEnRoute{Verb: "stance", ArrivesAt: arrives}); got != "Order on its way: stance change" {
		t.Errorf("no-cadence line = %q", got)
	}
}
