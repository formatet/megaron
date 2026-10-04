package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sort"
	"strings"
	"testing"
)

const architectureModule = "formatet/megaron/server"

// g1Allowed is the executable owner of the G1 production-import contract.
// Every internal package, including subpackages, must be named explicitly.
// Test-only imports are intentionally excluded; changing an edge is a design decision.
var g1Allowed = map[string][]string{
	"agora": {}, "ai": {}, "auth": {}, "clock": {}, "gossip": {}, "hexgrid": {},
	"movement": {}, "notify": {}, "religion": {}, "unit": {},
	"unit/shipnames": {}, "world": {},
	"province": {"hexgrid"},
	"events":   {"clock"}, "tick": {"clock", "events"},
	"chronicle": {"events"}, "settlement": {"province"},
	"economy":      {"clock", "events", "gossip", "hexgrid", "province"},
	"transport":    {"clock", "events", "province"},
	"capabilities": {"clock", "province", "religion", "unit"},
	"kharis":       {"ai", "clock", "economy", "events", "hexgrid", "religion", "unit"},
	"loyalty":      {"clock", "economy", "events", "settlement", "tick"},
	"combat":       {"capabilities", "clock", "economy", "events", "gossip", "hexgrid", "loyalty", "movement", "province", "tick", "transport", "unit"},
	"messenger":    {"ai", "clock", "gossip", "hexgrid", "movement", "province", "religion", "unit", "unit/shipnames", "world", "events", "tick", "chronicle", "settlement", "economy", "transport", "capabilities", "kharis", "loyalty", "combat"},
}

type g1Package struct {
	ImportPath string
	Imports    []string
}

func g1Violations(packages []g1Package) []string {
	var failures []string
	prefix := architectureModule + "/internal/"
	for _, p := range packages {
		source, internal := strings.CutPrefix(p.ImportPath, prefix)
		allowed, known := g1Allowed[source]
		if internal && !known {
			failures = append(failures, "unclassified internal package: "+source)
		}
		for _, target := range p.Imports {
			if internal && (strings.HasPrefix(target, architectureModule+"/api/") || strings.HasPrefix(target, architectureModule+"/cmd/")) {
				failures = append(failures, fmt.Sprintf("%s imports protocol adapter %s", source, target))
			}
			dependency, local := strings.CutPrefix(target, prefix)
			if !local {
				continue
			}
			if _, classified := g1Allowed[dependency]; !classified {
				failures = append(failures, "unclassified internal dependency: "+dependency)
			}
			if dependency == "notify" && p.ImportPath != architectureModule+"/api/handlers" && p.ImportPath != architectureModule+"/cmd/server" {
				failures = append(failures, p.ImportPath+" may not import notify")
			}
			if !internal || !known {
				continue
			}
			found := false
			for _, a := range allowed {
				if a == dependency {
					found = true
					break
				}
			}
			if !found {
				failures = append(failures, fmt.Sprintf("forbidden G1 edge: %s -> %s", source, dependency))
			}
		}
	}
	sort.Strings(failures)
	return failures
}

func TestG1ProductionImports(t *testing.T) {
	cmd := exec.Command("go", "list", "-json", "./...")
	cmd.Dir = "../.."
	output, err := cmd.Output()
	if err != nil {
		if e, ok := err.(*exec.ExitError); ok {
			t.Fatalf("go list: %v\n%s", err, e.Stderr)
		}
		t.Fatalf("go list: %v", err)
	}
	decoder := json.NewDecoder(strings.NewReader(string(output)))
	var packages []g1Package
	for {
		var p g1Package
		err := decoder.Decode(&p)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("decode go list: %v", err)
		}
		packages = append(packages, p)
	}
	if len(packages) == 0 {
		t.Fatal("go list returned no packages")
	}
	if failures := g1Violations(packages); len(failures) != 0 {
		t.Fatal(strings.Join(failures, "\n"))
	}
}

func TestG1Validator(t *testing.T) {
	tests := []struct {
		name, source, target string
		valid                bool
	}{
		{"geometry owner", "internal/province", "internal/hexgrid", true},
		{"economy routing consumer", "internal/economy", "internal/province", true},
		{"shipnames classified", "internal/unit/shipnames", "encoding/csv", true},
		{"handler hub adapter", "api/handlers", "internal/notify", true},
		{"server hub adapter", "cmd/server", "internal/notify", true},
		{"unknown package", "internal/g1unknown", "fmt", false},
		{"unknown subpackage", "internal/unit/newpackage", "fmt", false},
		{"unknown dependency", "internal/ai", "internal/g1unknown", false},
		{"upward edge", "internal/province", "internal/economy", false},
		{"transport combat", "internal/transport", "internal/combat", false},
		{"domain handler", "internal/ai", "api/handlers", false},
		{"domain command", "internal/ai", "cmd/keryx", false},
		{"domain hub", "internal/ai", "internal/notify", false},
		{"messenger auth", "internal/messenger", "internal/auth", false},
		{"other command hub", "cmd/keryx", "internal/notify", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			target := tc.target
			if strings.HasPrefix(target, "internal/") || strings.HasPrefix(target, "api/") || strings.HasPrefix(target, "cmd/") {
				target = architectureModule + "/" + target
			}
			p := g1Package{ImportPath: architectureModule + "/" + tc.source, Imports: []string{target}}
			failures := g1Violations([]g1Package{p})
			if (len(failures) == 0) != tc.valid {
				t.Fatalf("valid=%v; violations=%v", tc.valid, failures)
			}
		})
	}
}
