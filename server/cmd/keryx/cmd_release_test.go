package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func resetRelease(t *testing.T) {
	t.Helper()
	prevCommit, prevLatest, prevWarned := buildCommit, latestKeryx, newerWarned
	t.Cleanup(func() { buildCommit, latestKeryx, newerWarned = prevCommit, prevLatest, prevWarned })
	latestKeryx, newerWarned = "", false
}

func TestNewerKeryxLine(t *testing.T) {
	cases := []struct {
		own, latest string
		want        bool
	}{
		{"abc1234", "def5678", true},
		{"abc1234", "abc1234", false},
		{"abc1234", "abc1234ffffffff", false}, // the tool may be given a longer hash
		{"abc1234", "", false},
		{"dev", "def5678", false},
	}
	for _, c := range cases {
		got := newerKeryxLine(c.own, c.latest)
		if (got != "") != c.want {
			t.Errorf("own=%q latest=%q: line=%q, want line=%v", c.own, c.latest, got, c.want)
		}
	}
	line := newerKeryxLine("abc1234", "def5678")
	if line != "A newer keryx is out (def5678). Download: "+keryxDownloadURL {
		t.Errorf("line = %q", line)
	}
}

func TestKeryxDownloadURLMatchesCodex(t *testing.T) {
	b, err := os.ReadFile("../../../web/static/codex/keryx.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), keryxDownloadURL) {
		t.Errorf("Codex keryx.md does not link %s", keryxDownloadURL)
	}
}

func TestClientRemembersLatestHeader(t *testing.T) {
	resetRelease(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Keryx-Latest", "def5678")
		io.WriteString(w, `{}`)
	}))
	defer srv.Close()
	c := newClient(&Config{Server: srv.URL, Token: "t"})
	if _, err := c.get("/api/v1/x"); err != nil {
		t.Fatal(err)
	}
	if latestKeryx != "def5678" {
		t.Errorf("latestKeryx = %q", latestKeryx)
	}
}

// The notice goes to stderr only (agents parse stdout) and at most once.
func TestWarnNewerKeryxGoesToStderrOnceNeverStdout(t *testing.T) {
	resetRelease(t)
	buildCommit, latestKeryx = "abc1234", "def5678"
	oldOut, oldErr := os.Stdout, os.Stderr
	defer func() { os.Stdout, os.Stderr = oldOut, oldErr }()
	ro, wo, _ := os.Pipe()
	re, we, _ := os.Pipe()
	os.Stdout, os.Stderr = wo, we
	warnNewerKeryx()
	warnNewerKeryx()
	wo.Close()
	we.Close()
	out, _ := io.ReadAll(ro)
	errOut, _ := io.ReadAll(re)
	if len(out) != 0 {
		t.Errorf("stdout must stay untouched, got %q", out)
	}
	if strings.Count(string(errOut), "A newer keryx is out (def5678)") != 1 {
		t.Errorf("stderr = %q, want the line exactly once", errOut)
	}
}

func TestWarnNewerKeryxQuietWhenCurrentOrDev(t *testing.T) {
	for _, own := range []string{"def5678", "dev"} {
		resetRelease(t)
		buildCommit, latestKeryx = own, "def5678"
		oldErr := os.Stderr
		re, we, _ := os.Pipe()
		os.Stderr = we
		warnNewerKeryx()
		we.Close()
		os.Stderr = oldErr
		if b, _ := io.ReadAll(re); len(b) != 0 {
			t.Errorf("own=%q: expected silence, got %q", own, b)
		}
	}
}

func TestVersionCommandPrintsOwnVersionWithoutLogin(t *testing.T) {
	resetRelease(t)
	buildCommit = "abc1234"
	t.Setenv("POLEIA_CONFIG", t.TempDir()+"/none.json")
	root := newRootCommand()
	root.SetArgs([]string{"version"})
	out, err := captureStdout(t, root.Execute)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != "abc1234" {
		t.Errorf("version output = %q", out)
	}
}

func TestKeryxVersionFallsBackToDev(t *testing.T) {
	resetRelease(t)
	buildCommit = ""
	if got := keryxVersion(); got != "dev" { // test binaries carry no vcs.revision
		t.Errorf("version = %q, want dev", got)
	}
}

func TestKeryxReleasedNoticeText(t *testing.T) {
	raw := []byte(`{"commit":"def5678","url":"https://example.org/kx"}`)
	out := capturePrint(t, func() { printNotificationSummary(nil, notificationItem{Kind: "KeryxReleased", Level: 3, Body: raw}) })
	if !strings.Contains(out, "A new keryx is out (def5678). Download: https://example.org/kx") {
		t.Fatalf("got %q", out)
	}
}

func TestDossierShowsKeryxReleasedUnderSinceNotUrgent(t *testing.T) {
	notifs := notifJSON(1, "KeryxReleased", 99, `{"commit":"def5678","url":"https://example.org/kx"}`)
	newDossierFake(t, fullDossierReplies(notifs))
	text := buildDossier(newClient(cfg), cfg.WorldID, dossierNow)
	_, bodies := sectionBodies(t, text)
	if !strings.Contains(bodies["Since you last read"], "KeryxReleased") {
		t.Errorf("Since you last read lacks KeryxReleased:\n%s", bodies["Since you last read"])
	}
	if strings.Contains(bodies["Urgent"], "KeryxReleased") {
		t.Errorf("KeryxReleased must not be Urgent")
	}
}
