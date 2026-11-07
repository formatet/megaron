package main

import (
	"fmt"
	"os"
	"runtime/debug"
	"strings"

	"github.com/spf13/cobra"
)

// keryxDownloadURL is where the released client lives (one tag, replaced each
// release); the Codex article keryx.md links to the same address.
const keryxDownloadURL = "https://github.com/formatet/megaron/releases/download/keryx-alfa30/kx"

// buildCommit may be stamped with -ldflags "-X main.buildCommit=<commit>" for
// a build without VCS info; otherwise Go's own vcs.revision is used.
var buildCommit = ""

// latestKeryx is the newest released commit the server last announced in the
// X-Keryx-Latest response header ("" when it sent none).
var latestKeryx string

// newerWarned makes the notice appear at most once per run.
var newerWarned bool

// keryxVersion is the commit this binary was built from, 7 characters, or "dev".
func keryxVersion() string {
	if buildCommit != "" {
		return buildCommit
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, s := range info.Settings {
			if s.Key == "vcs.revision" && len(s.Value) >= 7 {
				return s.Value[:7]
			}
		}
	}
	return "dev"
}

// newerKeryxLine is the one-line notice, or "" when there is nothing to say:
// the server named no release, this is a dev build, or it is already current.
func newerKeryxLine(own, latest string) string {
	if latest == "" || own == "dev" || strings.HasPrefix(latest, own) || strings.HasPrefix(own, latest) {
		return ""
	}
	return fmt.Sprintf("A newer keryx is out (%s). Download: %s", latest, keryxDownloadURL)
}

// warnNewerKeryx writes the notice on stderr — never stdout, which agents parse.
func warnNewerKeryx() {
	if newerWarned {
		return
	}
	if line := newerKeryxLine(keryxVersion(), latestKeryx); line != "" {
		newerWarned = true
		fmt.Fprintln(os.Stderr, line)
	}
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the commit this keryx was built from",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println(keryxVersion())
			return nil
		},
	}
}
