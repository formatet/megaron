package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestRegisterRefusedWhenRegistrationClosed: with registration closed, Register
// answers 403 with a readable error before touching the auth service (nil here,
// so reaching it would panic). The web login page shows data.error verbatim.
func TestRegisterRefusedWhenRegistrationClosed(t *testing.T) {
	h := NewAuthHandler(nil)
	h.SetRegistrationClosed(true)
	rec := httptest.NewRecorder()
	h.Register(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/register",
		strings.NewReader(`{"username":"newcomer","password":"secret-password"}`)))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("Register with registration closed = %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "registration is closed") {
		t.Fatalf("error body = %s", rec.Body.String())
	}
}
