package main

import (
	"os"
	"strconv"
	"testing"
)

// TestEnvMapDim covers the MAP_WIDTH/MAP_HEIGHT parsing used by ensureWorld's
// fallback world seed (see the doc comment on ensureWorld) — default-on-unset,
// a valid override, and refusal (not silent clamping) on a too-small or
// non-integer value.
func TestEnvMapDim(t *testing.T) {
	const key = "MG_TEST_MAP_DIM"

	t.Run("unset falls back to default", func(t *testing.T) {
		os.Unsetenv(key)
		got, err := envMapDim(key, 56, minMapWidth)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != 56 {
			t.Errorf("got %d, want default 56", got)
		}
	})

	t.Run("valid override is honoured", func(t *testing.T) {
		t.Setenv(key, "80")
		got, err := envMapDim(key, 56, minMapWidth)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != 80 {
			t.Errorf("got %d, want 80", got)
		}
	})

	t.Run("too small refuses instead of clamping", func(t *testing.T) {
		t.Setenv(key, "5")
		_, err := envMapDim(key, 56, minMapWidth)
		if err == nil {
			t.Fatal("expected an error for a below-minimum value, got nil")
		}
	})

	t.Run("non-integer refuses", func(t *testing.T) {
		t.Setenv(key, "not-a-number")
		_, err := envMapDim(key, 56, minMapWidth)
		if err == nil {
			t.Fatal("expected an error for a non-integer value, got nil")
		}
	})

	t.Run("locked default matches the map spec", func(t *testing.T) {
		os.Unsetenv(key)
		w, err := envMapDim("MAP_WIDTH_UNSET_"+key, 56, minMapWidth)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		h, err := envMapDim("MAP_HEIGHT_UNSET_"+key, 40, minMapHeight)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if w != 56 || h != 40 {
			t.Errorf("got %dx%d, want locked default 56x40", w, h)
		}
	})

	t.Run("boundary at minimum is accepted", func(t *testing.T) {
		t.Setenv(key, strconv.Itoa(minMapWidth))
		got, err := envMapDim(key, 56, minMapWidth)
		if err != nil {
			t.Fatalf("unexpected error at exact minimum: %v", err)
		}
		if got != minMapWidth {
			t.Errorf("got %d, want %d", got, minMapWidth)
		}
	})
}
