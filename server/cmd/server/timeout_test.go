package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRequestTimeoutMiddleware(t *testing.T) {
	for _, path := range []string{"/ws/world", "/ws/", "/api/v1/worlds", "/api/ws/world", "/ws", "/ws-other", "/static/map.js", "/healthz", "/"} {
		t.Run(path, func(t *testing.T) {
			r := buildTestRouter() // The actual installMiddleware chain, including compression.
			wantDeadline := path != "/ws/world" && path != "/ws/"
			called := false
			before := time.Now()
			r.Get(path, func(w http.ResponseWriter, req *http.Request) {
				called = true
				deadline, ok := req.Context().Deadline()
				if !wantDeadline {
					if ok {
						t.Errorf("/ws/ request must have no middleware deadline; got %v", deadline)
					}
				} else {
					after := time.Now()
					if !ok {
						t.Error("non-WS request must retain its 30s deadline")
					} else if deadline.Before(before.Add(30*time.Second)) || deadline.After(after.Add(30*time.Second)) {
						t.Errorf("non-WS deadline = %v, want exactly 30s from middleware entry", deadline)
					}
				}
				w.WriteHeader(http.StatusNoContent)
			})
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
			if !called || rec.Code != http.StatusNoContent {
				t.Fatalf("handler called=%v, status=%d", called, rec.Code)
			}
		})
	}
}
