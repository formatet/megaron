package main

import (
	"strings"
	"testing"
)

// An expired session must tell a CLI player how to get back in — the server's
// own 401 text says "reload the page", which is the web's way, not keryx's.
func TestAPIError_Unauthorized_PointsAtKeryxLogin(t *testing.T) {
	body := []byte(`{"error":"Your session has expired — reload the page to log in again."}`)
	msg := apiError(body, 401).Error()
	if !strings.Contains(msg, "keryx login") {
		t.Errorf("401 error %q does not name keryx login", msg)
	}
	if strings.Contains(msg, "reload the page") {
		t.Errorf("401 error %q carries the web's instruction into the terminal", msg)
	}
	// Other statuses still pass the server's own words through.
	if got := apiError([]byte(`{"error":"not your province"}`), 403).Error(); got != "not your province (HTTP 403)" {
		t.Errorf("403 passthrough = %q", got)
	}
}
