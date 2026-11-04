package main

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestMessengerAwaitingPassageNamesPassageCommand(t *testing.T) {
	round3Client(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost:
			fmt.Fprint(w, `{"id":"m-123","arrives_at":"2026-11-04T18:34:00Z","passage_status":"awaiting_passage","passage_port":"Byblos"}`)
		case strings.HasSuffix(r.URL.Path, "/provinces"):
			fmt.Fprint(w, `[{"id":"default","own":true,"settlement_id":"own-1","name":"Home"},{"id":"p2","settlement_id":"dest-1","name":"Korinth"}]`)
		default:
			fmt.Fprint(w, `[]`)
		}
	})
	cmd := messengerCmd()
	cmd.SetArgs([]string{"--to", "Korinth", "--message", "hello"})
	out, err := captureStdout(t, cmd.Execute)
	if err != nil || !strings.Contains(out, "keryx passage --id m-123 --ship <id>") || !strings.Contains(out, "waiting in Byblos") {
		t.Fatalf("err %v out %q", err, out)
	}
}
