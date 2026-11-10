package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

const warnText = "Hull 2 of 5: any storm this ship meets will sink it (a storm takes 2 hull)."

func TestTransferBySeaPrintsStormWarningInPreviewAndReceipt(t *testing.T) {
	round3Client(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/trade/preview"):
			json.NewEncoder(w).Encode(map[string]any{"category": "naval", "ship_name": "Wavecutter", "ship_type": "galley", "cargo_capacity": 30.0, "storm_warning": warnText})
		case r.Method == "GET":
			json.NewEncoder(w).Encode([]map[string]any{{"settlement_id": "dest", "name": "Nostos", "own": true}})
		default:
			json.NewEncoder(w).Encode(map[string]any{"kind": "transfer", "ship_name": "Wavecutter", "storm_warning": warnText})
		}
	})
	cmd := transferCmd()
	cmd.SetArgs([]string{"--dest", "Nostos", "--good", "grain", "--qty", "3", "--by", "sea"})
	out, err := captureStdout(t, cmd.Execute)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(out, "⚠ "+warnText) != 2 {
		t.Fatalf("want the warning in preview and receipt:\n%s", out)
	}
}

func TestPassageReceiptPrintsStormWarning(t *testing.T) {
	round3Client(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"unit_id": "u1", "arrives_at": "2026-11-04T18:34:00Z", "storm_warning": warnText})
	})
	cmd := passageCmd()
	cmd.SetArgs([]string{"--id", "m1", "--ship", "cccccccc-1111-2222-3333-444444444444"})
	out, err := captureStdout(t, cmd.Execute)
	if err != nil || !strings.Contains(out, "⚠ "+warnText) {
		t.Fatalf("err %v out %q", err, out)
	}
}
