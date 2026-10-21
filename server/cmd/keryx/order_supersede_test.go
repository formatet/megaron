package main

import (
	"io"
	"os"
	"strings"
	"testing"
)

func TestOrderDispatchReceiptReportsPhysicalCallback(t *testing.T) {
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	defer func() { os.Stdout = old }()
	printSupersededOrderNote(map[string]any{"superseded_messenger_ids": []any{"old"}})
	w.Close()
	got, _ := io.ReadAll(r)
	r.Close()
	if !strings.Contains(string(got), "Earlier unboarded order withdrawn; its Runner is walking home") {
		t.Fatalf("receipt: %s", got)
	}
}
