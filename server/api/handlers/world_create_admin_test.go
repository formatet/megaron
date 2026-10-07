package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// POST /worlds archives the active world, so a logged-in player without the
// admin key must be refused before anything touches the database (nil pool:
// reaching the DB would panic, which this test reports as a failure).
func TestWorldCreateRequiresAdminKey(t *testing.T) {
	cases := []struct {
		name, envKey, header string
	}{
		{"no key configured, none sent", "", ""},
		{"key configured, none sent", "s3cret", ""},
		{"key configured, wrong key", "s3cret", "nope"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("POLEIA_ADMIN_KEY", c.envKey)
			h := &WorldHandler{}
			req := httptest.NewRequest(http.MethodPost, "/api/v1/worlds", strings.NewReader(`{"name":"x"}`))
			if c.header != "" {
				req.Header.Set("X-Admin-Key", c.header)
			}
			rec := httptest.NewRecorder()
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Fatalf("Create reached the database without a valid admin key: %v", r)
					}
				}()
				h.Create(rec, req)
			}()
			if rec.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want 403", rec.Code)
			}
		})
	}
}
