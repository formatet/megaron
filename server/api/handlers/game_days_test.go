package handlers

import (
	"fmt"
	"formatet/megaron/server/internal/tick"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestGameDayAlphaPageUsesRuntimeCadence(t *testing.T) {
	original := tick.TickSeconds
	defer func() { tick.TickSeconds = original }()
	root := filepath.Join("..", "..", "..")
	h, err := NewWebHandler(nil, nil, filepath.Join(root, "web/templates"), filepath.Join(root, "web/static"), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, seconds := range []int{1800, 3600, 6} {
		t.Run(fmt.Sprint(seconds), func(t *testing.T) {
			tick.TickSeconds = seconds
			rec := httptest.NewRecorder()
			h.AlfatestInfo(rec, httptest.NewRequest("GET", "/alfatestinfo", nil))
			want := "A game day passes every " + openingDayLength(seconds) + " in this world."
			if rec.Code != 200 || !strings.Contains(rec.Body.String(), want) {
				t.Fatalf("status%d want%q in%s", rec.Code, want, rec.Body.String())
			}
		})
	}
}
