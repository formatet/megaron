package main

import (
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
)

func TestStalledOrderNoticePrintsSealedInstruction(t *testing.T) {
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	defer func() { os.Stdout = old }()
	raw, _ := json.Marshal(map[string]any{"messenger_id": "own", "port_name": "Vasiliki", "kind": "order", "verb": "march", "order_summary": "Third Spearmen: March to (37,21); mission: colonize"})
	printPassageStalledLine(notificationItem{Kind: "PassageStalled", Body: raw})
	w.Close()
	got, _ := io.ReadAll(r)
	r.Close()
	if !strings.Contains(string(got), "Sealed order: Third Spearmen: March to (37,21); mission: colonize") {
		t.Fatalf("CLI: %s", got)
	}
}
