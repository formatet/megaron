package main

import (
	"os/exec"
	"path/filepath"
	"testing"
)

func TestDayPlayerLanguageGuard(t *testing.T) {
	script, err := filepath.Abs("../../../tools/day_guard.py")
	if err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("python3", script, "keryx").CombinedOutput()
	if err != nil {
		t.Fatalf("Day guard keryx: %v\n%s", err, out)
	}
}
