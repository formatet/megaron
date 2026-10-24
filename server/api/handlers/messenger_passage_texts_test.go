package handlers

// A3 (megaron_plan_budvagar_och_murtak_20261021): a refused passage says what,
// why and what to do, by name — live finding: a ship still being built got
// "the ship must be docked and free of missions", true but silent on when.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestArrangePassage_RefusalsNameShipAndWait(t *testing.T) {
	f := setupPassageArrangeFixture(t)
	homeID := f.settlement(t, "Phalasarna", 0, f.initiatorID, true)
	otherOwnID := f.settlement(t, "Kydonia", 10, f.initiatorID, true)
	foreignID := f.settlement(t, "Amnissos", 5, f.counterpartyID, true)
	for q := 1; q <= 4; q++ {
		f.mapTile(t, q, 0, "coastal_sea")
	}
	if code, _ := f.post(t, f.initiatorToken,
		"/worlds/"+f.worldID.String()+"/settlements/"+homeID.String()+"/messengers",
		map[string]any{"destination_id": foreignID.String(), "message": "hello"}); code != 201 {
		t.Fatalf("Send status = %d, want 201", code)
	}
	var messengerID uuid.UUID
	if err := f.pool.QueryRow(context.Background(),
		`SELECT id FROM messengers WHERE origin_id = $1 AND destination_id = $2`, homeID, foreignID,
	).Scan(&messengerID); err != nil {
		t.Fatal(err)
	}
	refusal := func(t *testing.T, ship uuid.UUID) string {
		t.Helper()
		code, resp := f.post(t, f.initiatorToken,
			"/worlds/"+f.worldID.String()+"/messengers/"+messengerID.String()+"/passage",
			map[string]any{"ship_id": ship.String()})
		if code < 400 {
			t.Fatalf("passage accepted (%d), want a refusal", code)
		}
		msg, _ := resp["error"].(string)
		if strings.Contains(msg, ship.String()) {
			t.Errorf("refusal shows a uuid: %q", msg)
		}
		return msg
	}

	t.Run("still being built says how long", func(t *testing.T) {
		ship := f.ship(t, homeID, f.initiatorID, "merchantman")
		if _, err := f.pool.Exec(context.Background(),
			`UPDATE units SET status='forming', name='Silent Galley', build_complete_at=$2 WHERE id=$1`,
			ship, f.clk.Now().Add(90*time.Second)); err != nil {
			t.Fatal(err)
		}
		msg := refusal(t, ship)
		for _, want := range []string{"Silent Galley", "still being built", "game day", "Arrange passage once"} {
			if !strings.Contains(msg, want) {
				t.Errorf("refusal %q lacks %q", msg, want)
			}
		}
	})

	t.Run("wrong own port names both ports", func(t *testing.T) {
		ship := f.ship(t, otherOwnID, f.initiatorID, "merchantman")
		msg := refusal(t, ship)
		for _, want := range []string{"Kydonia", "Phalasarna"} {
			if !strings.Contains(msg, want) {
				t.Errorf("refusal %q lacks %q", msg, want)
			}
		}
	})

	t.Run("another Wanax's ship points at automatic boarding", func(t *testing.T) {
		ship := f.ship(t, foreignID, f.counterpartyID, "merchantman")
		msg := refusal(t, ship)
		if !strings.Contains(msg, "another Wanax") || !strings.Contains(msg, "by itself") {
			t.Errorf("refusal %q does not say the runner boards others' ships by itself", msg)
		}
	})
}
