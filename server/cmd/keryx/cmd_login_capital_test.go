package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Haiku-8: login saved the colony as default because it came first in the markers,
// while every command's help says "default: your capital".
func TestAutoDetectProvincePrefersCapital(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]any{
			{"id": "foreign", "own": false, "is_capital": true},
			{"id": "colony", "own": true, "is_capital": false},
			{"id": "capital", "own": true, "is_capital": true},
		})
	}))
	defer srv.Close()
	c := &Client{server: srv.URL, token: "token", http: srv.Client()}
	if got := autoDetectProvince(c, "world"); got != "capital" {
		t.Fatalf("autoDetectProvince = %q, want the own capital", got)
	}
}
