package main

import (
	"os"
	"regexp"
	"testing"
)

// TestNoAnonymousFogOfWarReads guards the 2026-09-25 leak: map-shaped reads
// under /worlds/{worldID}/ were mounted with auth.OptionalMiddleware, and an
// anonymous caller got the no-eyes branch — /map returned every tile live with
// its deposits, /provinces every city with owner and position. The router is
// built inline in main(), so this reads the route table itself: the ONLY
// route allowed to accept an anonymous caller is the WebSocket (world-wide
// broadcasts, see the comment above it). A new OptionalMiddleware route turns
// this red and has to argue its case here.
func TestNoAnonymousFogOfWarReads(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`auth\.OptionalMiddleware\(authSvc\)\)\.(Get|Post|Put|Delete)\("([^"]+)"`)
	// "/" is the start page: it renders the sign-in form (no world data at all) or, for a caller whose
	// page cookie is valid, redirects to /play (2026-11-13). It reads no map, province or unit.
	allowed := map[string]bool{"/ws/{worldID}": true, "/": true}
	for _, m := range re.FindAllStringSubmatch(string(src), -1) {
		if !allowed[m[2]] {
			t.Errorf("route %s %s accepts anonymous callers (OptionalMiddleware) — fog-of-war reads must require a token", m[1], m[2])
		}
	}
	for _, p := range []string{"/worlds/{worldID}/map", "/worlds/{worldID}/storms", "/worlds/{worldID}/provinces", "/worlds/{worldID}/colonize-preview"} {
		if !regexp.MustCompile(`auth\.Middleware\(authSvc\)\)\.Get\("` + regexp.QuoteMeta(p) + `"`).Match(src) {
			t.Errorf("%s must be mounted with auth.Middleware", p)
		}
	}
}
