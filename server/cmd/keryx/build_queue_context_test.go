package main

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestBuildQueuePrintsCityAndHex(t *testing.T) {
	round3Client(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/provinces/") {
			fmt.Fprint(w, `{"settlement":{"name":"Tylissos","build_queue":[{"id":"q","type":"lumbermill","name":"Lumbermill","hex_q":0,"hex_r":7,"complete_at":"2026-10-26T12:00:00Z"}]}}`)
		} else {
			fmt.Fprint(w, `{"tick_seconds":60}`)
		}
	})
	out, err := captureStdout(t, func() error { return printBuildQueue(newClient(cfg), "world", "default") })
	if err != nil || !strings.Contains(out, "Lumbermill") || !strings.Contains(out, "Tylissos at (0,7)") {
		t.Fatalf("%q %v", out, err)
	}
}
