package handlers

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"formatet/megaron/server/internal/auth"
)

// The "pool empty" 409 must tell the player how to free a citizen, in words every surface can show;
// the terminal command is keryx's own hint (cmd/keryx refusalHint), never the server's.
func TestNoCitizensLeftMsgNamesTheFreeingVerbs(t *testing.T) {
	for _, want := range []string{"no citizens left in the pool", "take one off another place first"} {
		if !strings.Contains(noCitizensLeftMsg, want) {
			t.Fatalf("missing %q in %q", want, noCitizensLeftMsg)
		}
	}
	if strings.Contains(noCitizensLeftMsg, "keryx") {
		t.Fatalf("a server refusal must not name a keryx command: %q", noCitizensLeftMsg)
	}
}

// The expedition-budget preview refusal names the duration the player gave and the public range, never
// a route-specific minimum, and never says "too short" about a duration longer than the floor.
func TestExpeditionBudgetMsgNamesAskedDuration(t *testing.T) {
	got := expeditionBudgetMsg(10)
	if !strings.Contains(got, "an expedition of 10 game days is not enough") || !strings.Contains(got, "4 to 365 game days") || strings.Contains(got, "too short") {
		t.Fatal(got)
	}
	if def := expeditionBudgetMsg(0); !strings.Contains(def, "of 10 game days") {
		t.Fatal(def)
	}
}

func TestStormShipWarningByTypeAndHull(t *testing.T) {
	if got := stormShipWarning("galley", 2, 0); !strings.Contains(got, "will sink it") {
		t.Fatalf("damaged galley: %q", got)
	}
	if got := stormShipWarning("galley", 5, 0); got != "" {
		t.Fatalf("sound galley: %q", got)
	}
	if got := stormShipWarning("spearman", 1, 0); got != "" {
		t.Fatalf("land unit: %q", got)
	}
}

// The page cookie must outlive a 15-minute visit: it lives as long as the access token it carries
// (first-hour round two, item Q: a reload dropped the player to the sign-in page).
func TestTokenCookieLivesAsLongAsTheAccessToken(t *testing.T) {
	rec := httptest.NewRecorder()
	before := time.Now()
	setTokenCookie(rec, "jwt")
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies = %d", len(cookies))
	}
	if got := cookies[0].Expires.Sub(before); got < auth.AccessTokenTTL-time.Minute {
		t.Fatalf("cookie lives %v, want about %v", got, auth.AccessTokenTTL)
	}
}
