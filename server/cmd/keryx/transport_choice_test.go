package main

import (
	"strings"
	"testing"
)

func TestPrintTransportOptionsRoundsMinutesAndFormatsArrival(t *testing.T) {
	out, err := captureStdout(t, func() error {
		printTransportOptions([]transportOption{{By: "sea", Available: true, TravelTicks: 12, TravelSeconds: 21599.9999991, ArrivesAt: "2026-11-04T18:34:00Z", Risk: "storms"}}, "")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "about 360 minutes") || strings.Contains(out, "359.99") {
		t.Fatalf("minutes not rounded: %q", out)
	}
	if strings.Contains(out, "T18:34:00Z") {
		t.Fatalf("raw ISO leaked: %q", out)
	}
}
