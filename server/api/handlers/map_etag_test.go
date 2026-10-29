package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// An unchanged map poll is a 304 with no body; a changed one is a fresh 200.
func TestWriteJSONETag_UnchangedBodyIs304(t *testing.T) {
	get := func(v any, ifNoneMatch string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/map", nil)
		if ifNoneMatch != "" {
			req.Header.Set("If-None-Match", ifNoneMatch)
		}
		rec := httptest.NewRecorder()
		writeJSONETag(rec, req, v)
		return rec
	}
	fog := []map[string]any{{"q": 1, "r": 2, "terrain": "fog"}}
	first := get(fog, "")
	etag := first.Header().Get("ETag")
	if first.Code != http.StatusOK || etag == "" || first.Body.Len() == 0 {
		t.Fatalf("first poll: code %d etag %q body %d bytes", first.Code, etag, first.Body.Len())
	}
	if again := get(fog, etag); again.Code != http.StatusNotModified || again.Body.Len() != 0 {
		t.Fatalf("unchanged poll: code %d body %d bytes, want 304 and empty", again.Code, again.Body.Len())
	}
	seen := []map[string]any{{"q": 1, "r": 2, "terrain": "plains"}}
	changed := get(seen, etag)
	if changed.Code != http.StatusOK || changed.Header().Get("ETag") == etag {
		t.Fatalf("changed fog: code %d etag %q, want 200 with a new etag", changed.Code, changed.Header().Get("ETag"))
	}
}
