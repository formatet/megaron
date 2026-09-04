package main

import (
	"errors"
	"strings"
	"testing"
)

// TestResolvePassword covers the priority order chosen for `keryx login`:
// POLEIA_PASSWORD wins over the terminal prompt, and a script with no
// terminal and no POLEIA_PASSWORD gets a clear error instead of hanging on
// a prompt nobody can answer. There is deliberately no --password flag (it
// would leak into the process list and shell history), so this is the only
// surface to test.
func TestResolvePassword(t *testing.T) {
	t.Run("env wins over prompt", func(t *testing.T) {
		promptCalled := false
		got, err := resolvePassword("s3cret", true, func() (string, error) {
			promptCalled = true
			return "from-prompt", nil
		})
		if err != nil {
			t.Fatalf("resolvePassword() error = %v", err)
		}
		if got != "s3cret" {
			t.Errorf("resolvePassword() = %q, want %q", got, "s3cret")
		}
		if promptCalled {
			t.Error("resolvePassword() called the prompt even though POLEIA_PASSWORD was set")
		}
	})

	t.Run("falls back to prompt when env is empty and stdin is a terminal", func(t *testing.T) {
		got, err := resolvePassword("", true, func() (string, error) {
			return "typed-password", nil
		})
		if err != nil {
			t.Fatalf("resolvePassword() error = %v", err)
		}
		if got != "typed-password" {
			t.Errorf("resolvePassword() = %q, want %q", got, "typed-password")
		}
	})

	t.Run("no terminal and no env fails with a named-variable error instead of prompting", func(t *testing.T) {
		promptCalled := false
		_, err := resolvePassword("", false, func() (string, error) {
			promptCalled = true
			return "should not be called", nil
		})
		if err == nil {
			t.Fatal("resolvePassword() error = nil, want error naming POLEIA_PASSWORD")
		}
		if !strings.Contains(err.Error(), "POLEIA_PASSWORD") {
			t.Errorf("resolvePassword() error = %q, want it to name POLEIA_PASSWORD", err.Error())
		}
		if promptCalled {
			t.Error("resolvePassword() called the prompt despite no terminal being available")
		}
	})

	t.Run("propagates a prompt error", func(t *testing.T) {
		wantErr := errors.New("boom")
		_, err := resolvePassword("", true, func() (string, error) {
			return "", wantErr
		})
		if !errors.Is(err, wantErr) {
			t.Errorf("resolvePassword() error = %v, want %v", err, wantErr)
		}
	})
}
