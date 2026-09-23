package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// A 401 from the API middleware must be JSON with a human `error`: every web
// refusal surface renders the server's own error string, and a plain-text body
// reached a player whose tab outlived its token as "error 401".
func TestMiddleware_Unauthorized_IsJSONWithHumanMessage(t *testing.T) {
	h := Middleware(NewService(nil, "test-secret"))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler reached without a valid token")
	}))
	for name, authz := range map[string]string{
		"missing": "",
		"invalid": "Bearer not-a-jwt",
	} {
		req := httptest.NewRequest("GET", "/api/v1/anything", nil)
		if authz != "" {
			req.Header.Set("Authorization", authz)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s: status %d, want 401", name, rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
			t.Errorf("%s: Content-Type %q, want application/json", name, ct)
		}
		var body struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("%s: body %q is not JSON: %v", name, rec.Body.String(), err)
		}
		if body.Error != SessionExpiredMessage {
			t.Errorf("%s: error %q, want %q", name, body.Error, SessionExpiredMessage)
		}
	}
}
