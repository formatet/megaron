package main

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// Haiku round 6: "fish 95% / cult 15%" in one list read as 110% of the city.
func TestAllocate_DevotionIsApartFromTheWorkSplit(t *testing.T) {
	round3Client(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/goods") {
			fmt.Fprint(w, `[{"key":"fish","percent":95,"citizens":400,"labor_pool":422,"idle_citizens":0}]`)
		} else {
			fmt.Fprint(w, `{"settlement":{"devotion":0.15}}`)
		}
	})
	out, err := captureStdout(t, allocateCmd().Execute)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "cult (devotion)") {
		t.Errorf("devotion listed as a row of the work split:\n%s", out)
	}
	if !strings.Contains(out, "Temple devotion: 15% (63 people)") || !strings.Contains(out, "not taken from the split") {
		t.Errorf("devotion line missing:\n%s", out)
	}
}
