package main

import (
	"reflect"
	"testing"
)

// parsePositionals runs placeCmd's actual flag-parsing pipeline (ParseFlags,
// exactly the step where a bare "-1" gets misparsed as an unknown shorthand
// flag without SetInterspersed(false)) and returns the leftover positional
// tokens plus whatever cobra.RangeArgs(2,3) (cmd.Args) reports against them.
// This mirrors the two-stage pipeline cmd.Execute() runs before ever
// reaching RunE — no HTTP involved at either stage.
func parsePositionals(t *testing.T, tokens []string) ([]string, error) {
	t.Helper()
	cmd := placeCmd()
	if err := cmd.ParseFlags(tokens); err != nil {
		t.Fatalf("ParseFlags(%v) = %v, want no flag-parsing error — this is exactly what breaks if SetInterspersed(false) regresses", tokens, err)
	}
	args := cmd.Flags().Args()
	return args, cmd.Args(cmd, args)
}

// TestPlaceArgCountForms locks megaron_plan_gubbeflytt.md's §Ytval shape:
// the pre-existing two-argument call form must keep working byte-for-byte
// (acceptans #4's regression guard), the new delta argument must be
// accepted in both directions, and a stray fourth argument must still be
// rejected by cobra.RangeArgs(2,3).
func TestPlaceArgCountForms(t *testing.T) {
	tests := []struct {
		name       string
		tokens     []string
		wantArgs   []string
		wantErrSub string // "" means no error
	}{
		{
			name:     "two-arg form accepted (acceptans #4 regression guard)",
			tokens:   []string{"grain", "5"},
			wantArgs: []string{"grain", "5"},
		},
		{
			name:     "three-arg form with +n accepted",
			tokens:   []string{"grain", "5", "+2"},
			wantArgs: []string{"grain", "5", "+2"},
		},
		{
			name:     "three-arg form with -n accepted — bare -1 reaches RunE as a positional, not a flag",
			tokens:   []string{"grain", "5", "-1"},
			wantArgs: []string{"grain", "5", "-1"},
		},
		{
			name:       "four args rejected (RangeArgs(2,3) upper bound)",
			tokens:     []string{"grain", "5", "-1", "extra"},
			wantErrSub: "accepts between 2 and 3 arg",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args, err := parsePositionals(t, tt.tokens)
			if !reflect.DeepEqual(args, tt.wantArgs) && tt.wantErrSub == "" {
				t.Errorf("leftover positionals = %v, want %v", args, tt.wantArgs)
			}
			if tt.wantErrSub == "" {
				if err != nil {
					t.Errorf("cmd.Args(%v) = %v, want nil", args, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("cmd.Args(%v) = nil, want an error containing %q", args, tt.wantErrSub)
			}
			if !contains(err.Error(), tt.wantErrSub) {
				t.Errorf("cmd.Args(%v) error = %q, want it to contain %q", args, err.Error(), tt.wantErrSub)
			}
		})
	}
}

// TestPlaceDeltaRejection locks the delta-content validation inside
// placeCmd's RunE (cmd_place.go): a zero delta and a non-numeric delta must
// both be rejected, with the exact wording a player/agent sees. Both
// rejections happen before any HTTP call (ordinal parses fine, and the
// delta check runs before placeMany/unplaceMany are ever reached), so a
// bare non-nil *Config is enough — no network fixture needed.
func TestPlaceDeltaRejection(t *testing.T) {
	orig := cfg
	cfg = &Config{Server: "http://unused.invalid", WorldID: "world-1", ProvinceID: "prov-1"}
	t.Cleanup(func() { cfg = orig })

	tests := []struct {
		name    string
		delta   string
		wantErr string
	}{
		{"zero delta rejected", "0", `delta must be a non-zero signed number (e.g. +2 or -1), got "0"`},
		{"non-numeric delta rejected", "abc", `delta must be a non-zero signed number (e.g. +2 or -1), got "abc"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := placeCmd()
			err := cmd.RunE(cmd, []string{"grain", "5", tt.delta})
			if err == nil {
				t.Fatalf("RunE(delta=%q) = nil, want a rejection error", tt.delta)
			}
			if err.Error() != tt.wantErr {
				t.Errorf("RunE(delta=%q) error = %q, want %q", tt.delta, err.Error(), tt.wantErr)
			}
		})
	}
}
