package main

import (
	"bytes"
	"strings"
	"testing"
)

func rootHelpText(t *testing.T) string {
	t.Helper()
	root := newRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"--help"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func TestRootHelpIsAMap(t *testing.T) {
	out := rootHelpText(t)
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) > 45 {
		t.Fatalf("root help is %d lines, want <= 45", len(lines))
	}
	if strings.Contains(strings.ToLower(out), "kingdom") {
		t.Fatal("root help must not mention kingdoms")
	}
	order := []string{"Start here:", "Build and work:", "Move and fight:", "Goods and talk:", "Gods:", "Look closer:", "Account:"}
	last := -1
	for _, h := range order {
		i := strings.Index(out, "\n"+h+"\n")
		if i <= last {
			t.Fatalf("group %q missing or out of order", h)
		}
		last = i
	}
	n := len(lines)
	if lines[n-2] != "Each round: keryx dossier, then keryx actions." || lines[n-1] != "Every command has --help with copy-paste examples." {
		t.Fatalf("root help must end with the two fixed lines, got %q / %q", lines[n-2], lines[n-1])
	}
}

// Every top-level command is either listed in the help or deliberately hidden.
func TestEveryVisibleCommandIsInAGroup(t *testing.T) {
	root := newRootCommand()
	out := rootHelpText(t)
	for _, c := range root.Commands() {
		if c.Hidden || c.Name() == "help" {
			continue
		}
		if c.GroupID == "" {
			t.Errorf("visible command %q has no help group", c.Name())
		}
		if !strings.Contains(out, c.Name()) {
			t.Errorf("visible command %q missing from root help", c.Name())
		}
	}
}

// Hidden is not removed: every command, hidden or not, still resolves and
// answers --help.
func TestHiddenCommandsStillRun(t *testing.T) {
	root := newRootCommand()
	var names [][]string
	for _, c := range root.Commands() {
		names = append(names, []string{c.Name()})
		for _, s := range c.Commands() {
			names = append(names, []string{c.Name(), s.Name()})
		}
	}
	hidden := 0
	for _, path := range names {
		fresh := newRootCommand()
		found, _, err := fresh.Find(path)
		if err != nil || found == nil || found.Name() != path[len(path)-1] {
			t.Errorf("%v does not resolve: %v", path, err)
			continue
		}
		if found.Hidden {
			hidden++
		}
		var buf bytes.Buffer
		fresh.SetOut(&buf)
		fresh.SetErr(&buf)
		fresh.SetArgs(append(append([]string{}, path...), "--help"))
		if err := fresh.Execute(); err != nil {
			t.Errorf("%v --help failed: %v", path, err)
		}
	}
	if hidden < 20 {
		t.Errorf("expected the kingdom stubs and overlapping views to be hidden, only %d are", hidden)
	}
}
