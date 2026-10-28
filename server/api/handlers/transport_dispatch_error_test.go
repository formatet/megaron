package handlers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"formatet/megaron/server/internal/transport"
)

func TestTransportDispatchRefusalIs422WithPlayerText(t *testing.T) {
	rec := httptest.NewRecorder()
	writeTransportDispatchError(rec, fmt.Errorf("wrapped: %w", &transport.DispatchRefusal{Text: "Horns of Crete is not in harbour at Phaistos."}), "could not dispatch transfer caravan")
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "Horns of Crete is not in harbour at Phaistos.") {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	writeTransportDispatchError(rec, fmt.Errorf("db down"), "could not dispatch transfer caravan")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("server fault code=%d", rec.Code)
	}
}
