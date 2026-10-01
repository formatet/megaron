package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestMain points keryx's config at a throwaway file for the whole package.
// Several commands under test (join, settle, the stale-province self-heal)
// call saveConfig, and without this they wrote world-1/prov-1 and a dead
// httptest URL over the developer's real ~/.config/poleia/config.json on
// every suite run — found 2026-10-01. Tests that need their own config still
// set POLEIA_CONFIG with t.Setenv, which overrides this.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "keryx-test-config-")
	if err != nil {
		panic(err)
	}
	os.Setenv("POLEIA_CONFIG", filepath.Join(dir, "config.json"))
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}
