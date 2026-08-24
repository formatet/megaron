package main

import (
	"testing"

	"github.com/spf13/cobra"
)

// Rad I, megaron_plan_cli_sanning.md: `march` only took --q/--r, `redirect`
// only took --target "q,r" — two commands, same kind of target, two
// incompatible shapes. resolveTargetHex is the shared parser both commands
// now call; these tests exercise it directly (no network — RunE is never
// invoked) and then check both real commands actually wired the flags.

func newQRTargetTestCmd() (cmd *cobra.Command, target *string, q, r *int) {
	cmd = &cobra.Command{Use: "x"}
	target = new(string)
	q, r = new(int), new(int)
	cmd.Flags().StringVar(target, "target", "", "")
	cmd.Flags().IntVar(q, "q", 0, "")
	cmd.Flags().IntVar(r, "r", 0, "")
	return
}

func TestResolveTargetHex(t *testing.T) {
	t.Run("q and r given", func(t *testing.T) {
		cmd, target, q, r := newQRTargetTestCmd()
		if err := cmd.ParseFlags([]string{"--q", "5", "--r", "-3"}); err != nil {
			t.Fatal(err)
		}
		gotQ, gotR, given, err := resolveTargetHex(cmd, *target, *q, *r)
		if err != nil || !given || gotQ != 5 || gotR != -3 {
			t.Fatalf("got (%d,%d,given=%v,err=%v), want (5,-3,true,nil)", gotQ, gotR, given, err)
		}
	})

	t.Run("target given", func(t *testing.T) {
		cmd, target, q, r := newQRTargetTestCmd()
		if err := cmd.ParseFlags([]string{"--target", "5,-3"}); err != nil {
			t.Fatal(err)
		}
		gotQ, gotR, given, err := resolveTargetHex(cmd, *target, *q, *r)
		if err != nil || !given || gotQ != 5 || gotR != -3 {
			t.Fatalf("got (%d,%d,given=%v,err=%v), want (5,-3,true,nil)", gotQ, gotR, given, err)
		}
	})

	t.Run("neither given is not an error — caller decides", func(t *testing.T) {
		cmd, target, q, r := newQRTargetTestCmd()
		_, _, given, err := resolveTargetHex(cmd, *target, *q, *r)
		if err != nil || given {
			t.Fatalf("got given=%v err=%v, want given=false err=nil", given, err)
		}
	})

	t.Run("only q without r is an error", func(t *testing.T) {
		cmd, target, q, r := newQRTargetTestCmd()
		if err := cmd.ParseFlags([]string{"--q", "5"}); err != nil {
			t.Fatal(err)
		}
		if _, _, _, err := resolveTargetHex(cmd, *target, *q, *r); err == nil {
			t.Fatal("want error for --q without --r")
		}
	})

	t.Run("both --target and --q/--r together is an error, not a silent pick", func(t *testing.T) {
		cmd, target, q, r := newQRTargetTestCmd()
		if err := cmd.ParseFlags([]string{"--target", "5,-3", "--q", "1"}); err != nil {
			t.Fatal(err)
		}
		if _, _, _, err := resolveTargetHex(cmd, *target, *q, *r); err == nil {
			t.Fatal("want error when both forms are given")
		}
	})

	t.Run("invalid --target still reports a parse error", func(t *testing.T) {
		cmd, target, q, r := newQRTargetTestCmd()
		if err := cmd.ParseFlags([]string{"--target", "not-a-hex"}); err != nil {
			t.Fatal(err)
		}
		if _, _, _, err := resolveTargetHex(cmd, *target, *q, *r); err == nil {
			t.Fatal("want error for an unparseable --target")
		}
	})
}

// TestMarchAndRedirectBothAcceptBothForms is the acceptance check for row I:
// `unit march` and `unit redirect` both expose --q/--r AND --target, so
// neither command's existing form (agent configs use them) was removed.
func TestMarchAndRedirectBothAcceptBothForms(t *testing.T) {
	for _, tc := range []struct {
		name string
		cmd  *cobra.Command
	}{
		{"march", unitMarchCmd()},
		{"redirect", unitRedirectCmd()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, flag := range []string{"q", "r", "target"} {
				if tc.cmd.Flags().Lookup(flag) == nil {
					t.Errorf("keryx unit %s is missing --%s", tc.name, flag)
				}
			}
		})
	}
}
