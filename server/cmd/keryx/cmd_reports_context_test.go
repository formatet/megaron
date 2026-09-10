package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// The reading surface for player reports. The client attaches diagnostics to
// every report (web/static/js/megaron/ui/diagnostics.js); glued onto the report
// line as raw JSON — which is what happened before — that made every report an
// unreadable wall. These lock the rendering AND the promise that nothing in the
// free-form blob is ever silently dropped.

func TestReportContextLines_EmptyIsNothing(t *testing.T) {
	for _, raw := range []string{"", "null"} {
		if got := reportContextLines(json.RawMessage(raw)); got != nil {
			t.Errorf("reportContextLines(%q) = %v, want nil", raw, got)
		}
	}
}

func TestReportContextLines_ClientEnvironmentIsReadable(t *testing.T) {
	raw := `{"client":{"ua":"Mozilla/5.0 (X11; Linux x86_64) Chrome/140","viewport":"1920x1080","dpr":2,"lang":"sv-SE"}}`
	got := strings.Join(reportContextLines(json.RawMessage(raw)), "\n")
	// Reports 3d82447a / c12c29be were Chromium-specific layout bugs that had
	// to be diagnosed from prose; this is the line that answers them.
	for _, want := range []string{"Chrome/140", "1920x1080", "@2x", "sv-SE"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestReportContextLines_RefusalCarriesTheServersOwnWords(t *testing.T) {
	raw := `{"recent_api_failures":[{"path":"/api/v1/worlds/:id/provinces/:id/placements","status":409,"body":"no citizens left in the pool"}]}`
	got := strings.Join(reportContextLines(json.RawMessage(raw)), "\n")
	if !strings.Contains(got, "409") || !strings.Contains(got, "no citizens left in the pool") {
		t.Errorf("refusal not rendered:\n%s", got)
	}
	if !strings.Contains(got, "placements") {
		t.Errorf("endpoint not rendered:\n%s", got)
	}
}

func TestReportContextLines_JSErrorCarriesItsSource(t *testing.T) {
	raw := `{"recent_js_errors":[{"message":"x is not a function","source":"city.js:455"}]}`
	got := strings.Join(reportContextLines(json.RawMessage(raw)), "\n")
	if !strings.Contains(got, "x is not a function") || !strings.Contains(got, "city.js:455") {
		t.Errorf("js error not rendered:\n%s", got)
	}
}

// The blob is free-form by design (mig 123) and keeps gaining keys. An
// unrecognised field must survive to the reader, never be swallowed.
func TestReportContextLines_UnknownFieldsSurvive(t *testing.T) {
	raw := `{"client":{"ua":"UA","viewport":"800x600","dpr":1},"settlement_id":"abc-123","some_future_key":42}`
	got := strings.Join(reportContextLines(json.RawMessage(raw)), "\n")
	if !strings.Contains(got, "abc-123") {
		t.Errorf("settlement_id was dropped:\n%s", got)
	}
	if !strings.Contains(got, "some_future_key") {
		t.Errorf("an unrecognised key was dropped — the blob is free-form:\n%s", got)
	}
}

func TestReportContextLines_NonObjectIsShownVerbatimNotDropped(t *testing.T) {
	got := reportContextLines(json.RawMessage(`"just a string"`))
	if len(got) != 1 || !strings.Contains(got[0], "just a string") {
		t.Errorf("non-object context must still reach the reader, got %v", got)
	}
}

func TestReportContextLines_EveryBufferedFailureIsShown(t *testing.T) {
	raw := `{"recent_api_failures":[
		{"path":"/a","status":400,"body":"first"},
		{"path":"/b","status":409,"body":"second"},
		{"path":"/c","status":422,"body":"third"}]}`
	lines := reportContextLines(json.RawMessage(raw))
	for _, want := range []string{"first", "second", "third"} {
		found := false
		for _, l := range lines {
			if strings.Contains(l, want) {
				found = true
			}
		}
		if !found {
			t.Errorf("failure %q missing — the sequence is the diagnosis, not just the last one", want)
		}
	}
}
