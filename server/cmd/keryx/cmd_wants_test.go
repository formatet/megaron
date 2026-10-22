package main

import (
	"strings"
	"testing"
)

func TestWantsCommandRemoved(t *testing.T) {
	for _, args := range [][]string{{"wants"}, {"wants", "--json"}} {
		root := newRootCommand()
		root.SetArgs(args)
		if _, err := captureStdout(t, root.Execute); err == nil || !strings.Contains(err.Error(), `unknown command "wants"`) {
			t.Fatalf("%v: %v", args, err)
		}
	}
	root := newRootCommand()
	for _, cmd := range root.Commands() {
		if cmd.Name() == "wants" {
			t.Fatal("wants still registered")
		}
		for _, alias := range cmd.Aliases {
			if alias == "wants" {
				t.Fatal("wants alias still registered")
			}
		}
	}
	if strings.Contains(root.UsageString(), "wants") {
		t.Fatal("wants still in help")
	}
}
