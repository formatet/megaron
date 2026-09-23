package main

import (
	"errors"
	"strings"
	"testing"
)

func TestResolveNewPassword(t *testing.T) {
	answers := func(a ...string) func(string) (string, error) {
		i := 0
		return func(string) (string, error) { i++; return a[i-1], nil }
	}
	if got, err := resolveNewPassword("from-env", false, nil); err != nil || got != "from-env" {
		t.Errorf("env wins: %q, %v", got, err)
	}
	if _, err := resolveNewPassword("", false, nil); err == nil || !strings.Contains(err.Error(), "POLEIA_NEW_PASSWORD") {
		t.Errorf("no env, no tty: err = %v, want one naming POLEIA_NEW_PASSWORD", err)
	}
	if got, err := resolveNewPassword("", true, answers("abc", "abc")); err != nil || got != "abc" {
		t.Errorf("matching prompts: %q, %v", got, err)
	}
	if _, err := resolveNewPassword("", true, answers("abc", "abd")); err == nil {
		t.Error("differing prompts must refuse — a typo would lock the player out")
	}
	if _, err := resolveNewPassword("", true, func(string) (string, error) { return "", errors.New("eof") }); err == nil {
		t.Error("prompt error must propagate")
	}
}
