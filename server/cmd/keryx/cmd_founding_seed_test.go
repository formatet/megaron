package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFoundingPreviewKeepsFractionalCarriedGrain(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Query().Get("seed") != "17.25" || r.URL.Query().Get("pop") != "1000" {
			t.Errorf("wrong preview request %s %s", r.Method, r.URL)
		}
		_, _ = w.Write([]byte(`{"grain":{"seed":17.25}}`))
	}))
	defer server.Close()
	p, err := fetchColonizePreviewParams(newClient(&Config{Server: server.URL}), "world", 0, 0, 1000, 17.25)
	if err != nil {
		t.Fatal(err)
	}
	if p.Grain.Seed != 17.25 {
		t.Fatalf("forecast seed=%g want17.25", p.Grain.Seed)
	}
}
