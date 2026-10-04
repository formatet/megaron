package main

import (
	"github.com/go-chi/chi/v5"
	"net/http"
)

// The invitation is public; it must not inherit the play/world auth middleware.
func registerAlfatestInfoRoute(r chi.Router, page http.HandlerFunc) {
	r.Get("/alfatestinfo", page)
}
