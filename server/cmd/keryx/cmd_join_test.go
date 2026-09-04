package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// joinTestServer mocks the three endpoints the join/founding-status surface
// touches: /founding/status, /worlds/{id}/provinces (autoDetectProvince /
// neverJoined) and /worlds/{id}/join itself. statusBody and provincesBody
// feed the first two; joinBody feeds the third (only used by TestJoinCmd*).
func joinTestServer(t *testing.T, statusBody, provincesBody, joinBody string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		switch {
		case strings.HasSuffix(r.URL.Path, "/founding/status"):
			_, _ = w.Write([]byte(statusBody))
		case strings.HasSuffix(r.URL.Path, "/provinces"):
			_, _ = w.Write([]byte(provincesBody))
		case strings.HasSuffix(r.URL.Path, "/join"):
			_, _ = w.Write([]byte(joinBody))
		default:
			_, _ = w.Write([]byte(`{}`))
		}
	}))
}

// captureStdout runs fn with os.Stdout redirected and returns everything
// written to it — same harness shape as runIdleCmd (cmd_idle_test.go).
func captureStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	runErr := fn()
	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	buf.ReadFrom(r)
	return buf.String(), runErr
}

func withTestConfig(t *testing.T, serverURL string) {
	t.Helper()
	prevCfg, prevJSON := cfg, jsonMode
	cfg = &Config{Server: serverURL, WorldID: "world-1", Token: "t"}
	jsonMode = false
	t.Cleanup(func() { cfg, jsonMode = prevCfg, prevJSON })
}

// TestFoundingStatus_NeverJoined_NamesJoinVerb is (b) from
// megaron_plan_tva_gate1_slices §1: GET .../founding/status answers
// {"active":false} identically whether the player never joined or already
// founded their metropolis. A player who never joined must be told to run
// `keryx join`, NOT that "the metropolis is already founded" — that message
// sends them looking for a city that never existed.
func TestFoundingStatus_NeverJoined_NamesJoinVerb(t *testing.T) {
	ts := joinTestServer(t, `{"active":false}`, `[]`, "")
	defer ts.Close()
	withTestConfig(t, ts.URL)

	out, err := captureStdout(t, func() error {
		return foundingStatusCmd().RunE(nil, nil)
	})
	if err != nil {
		t.Fatalf("RunE: %v", err)
	}
	if !strings.Contains(out, "join") {
		t.Errorf("never-joined message must contain the word 'join':\n%s", out)
	}
	if strings.Contains(out, "already founded") {
		t.Errorf("never-joined message must NOT claim the metropolis is already founded:\n%s", out)
	}
}

// TestFoundingStatus_AlreadyFounded_KeepsExistingMessage is the negative pin:
// a player who genuinely holds a metropolis (owns a province) still gets the
// original "already founded" message, unchanged, and it must not also claim
// they never joined.
func TestFoundingStatus_AlreadyFounded_KeepsExistingMessage(t *testing.T) {
	ts := joinTestServer(t, `{"active":false}`, `[{"id":"prov-1","own":true}]`, "")
	defer ts.Close()
	withTestConfig(t, ts.URL)

	out, err := captureStdout(t, func() error {
		return foundingStatusCmd().RunE(nil, nil)
	})
	if err != nil {
		t.Fatalf("RunE: %v", err)
	}
	if !strings.Contains(out, "already founded") {
		t.Errorf("already-founded message regressed:\n%s", out)
	}
	if strings.Contains(out, "haven't joined") {
		t.Errorf("already-founded message must not also claim the player never joined:\n%s", out)
	}
}

// TestFoundingSettle_NeverJoined_NamesJoinVerb mirrors the status test for
// `founding settle`'s error branch (cmd_founding.go:187 before this fix).
func TestFoundingSettle_NeverJoined_NamesJoinVerb(t *testing.T) {
	ts := joinTestServer(t, `{"active":false}`, `[]`, "")
	defer ts.Close()
	withTestConfig(t, ts.URL)

	err := foundingSettleCmd().RunE(nil, nil)
	if err == nil {
		t.Fatal("RunE: want error, got nil")
	}
	if !strings.Contains(err.Error(), "join") {
		t.Errorf("never-joined error must contain the word 'join': %v", err)
	}
	if strings.Contains(err.Error(), "already founded") {
		t.Errorf("never-joined error must NOT claim the metropolis is already founded: %v", err)
	}
}

// TestFoundingSettle_AlreadyFounded_KeepsExistingMessage is the negative pin
// for `founding settle`.
func TestFoundingSettle_AlreadyFounded_KeepsExistingMessage(t *testing.T) {
	ts := joinTestServer(t, `{"active":false}`, `[{"id":"prov-1","own":true}]`, "")
	defer ts.Close()
	withTestConfig(t, ts.URL)

	err := foundingSettleCmd().RunE(nil, nil)
	if err == nil {
		t.Fatal("RunE: want error, got nil")
	}
	if !strings.Contains(err.Error(), "already founded") {
		t.Errorf("already-founded error regressed: %v", err)
	}
	if strings.Contains(err.Error(), "haven't joined") {
		t.Errorf("already-founded error must not also claim the player never joined: %v", err)
	}
}

// TestJoinCmd_FreshJoin_ReportsHostAndNextStep covers the 201 branch: a
// brand new host, no settlement yet — autoDetectProvince (via /provinces)
// correctly finds nothing to write into cfg.ProvinceID.
func TestJoinCmd_FreshJoin_ReportsHostAndNextStep(t *testing.T) {
	ts := joinTestServer(t, "", `[]`,
		`{"host_unit_id":"host-1","tile":{"Q":5,"R":9},"culture":"minoan","population":1000}`)
	defer ts.Close()
	withTestConfig(t, ts.URL)

	out, err := captureStdout(t, func() error {
		return joinCmd().RunE(nil, nil)
	})
	if err != nil {
		t.Fatalf("RunE: %v", err)
	}
	if !strings.Contains(out, "1000 people") {
		t.Errorf("output missing population:\n%s", out)
	}
	if !strings.Contains(out, "(5,9)") {
		t.Errorf("output missing tile coordinates:\n%s", out)
	}
	if !strings.Contains(out, "founding status") {
		t.Errorf("output missing the next-step pointer:\n%s", out)
	}
	if cfg.ProvinceID != "" {
		t.Errorf("cfg.ProvinceID = %q, want empty — a fresh host has no province yet", cfg.ProvinceID)
	}
}

// TestJoinCmd_ExistingSettlement_IsNotAnError covers the idempotent branch
// where the player already holds a metropolis: the server returns 200 with
// existing:true + province_id, and the CLI must report it, not error.
func TestJoinCmd_ExistingSettlement_IsNotAnError(t *testing.T) {
	ts := joinTestServer(t, "", `[{"id":"prov-1","own":true}]`,
		`{"province_id":"prov-1","existing":true}`)
	defer ts.Close()
	withTestConfig(t, ts.URL)

	out, err := captureStdout(t, func() error {
		return joinCmd().RunE(nil, nil)
	})
	if err != nil {
		t.Fatalf("RunE: %v", err)
	}
	if !strings.Contains(out, "already hold a metropolis") {
		t.Errorf("output missing the existing-settlement message:\n%s", out)
	}
	if cfg.ProvinceID != "prov-1" {
		t.Errorf("cfg.ProvinceID = %q, want %q — join should re-resolve it into config", cfg.ProvinceID, "prov-1")
	}
}

// TestJoinCmd_ExistingHost_IsNotAnError covers the idempotent branch where
// the player already has a wandering host (joined but not yet founded): the
// server returns 200 with existing:true + host_unit_id, no province_id.
func TestJoinCmd_ExistingHost_IsNotAnError(t *testing.T) {
	ts := joinTestServer(t, "", `[]`,
		`{"host_unit_id":"host-1","existing":true}`)
	defer ts.Close()
	withTestConfig(t, ts.URL)

	out, err := captureStdout(t, func() error {
		return joinCmd().RunE(nil, nil)
	})
	if err != nil {
		t.Fatalf("RunE: %v", err)
	}
	if !strings.Contains(out, "already have a wandering host") {
		t.Errorf("output missing the existing-host message:\n%s", out)
	}
}
