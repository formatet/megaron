package tick

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
	out, err := exec.Command("python3", script, "server").CombinedOutput()
	if err != nil {
		t.Fatalf("Day guard server: %v\n%s", err, out)
	}
}
