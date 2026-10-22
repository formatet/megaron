package main

import (
	"strings"
	"testing"
)

// Haiku round 5: a colony build failing on goods must name the way to move them.
func TestInsufficientGoodsNamesTransfer(t *testing.T) {
	err := apiError([]byte(`{"error":"insufficient_goods","missing":[{"good":"timber","need":20,"have":0}]}`), 422)
	if !strings.Contains(err.Error(), "keryx transfer") {
		t.Fatalf("insufficient_goods error does not name keryx transfer: %v", err)
	}
	other := apiError([]byte(`{"error":"insufficient_kharis","missing":[{"good":"kharis","need":20,"have":0}]}`), 422)
	if strings.Contains(other.Error(), "keryx transfer") {
		t.Fatalf("transfer hint leaked into another error: %v", other)
	}
}
