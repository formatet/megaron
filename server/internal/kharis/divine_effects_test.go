package kharis

// Proof tests for megaron_plan_tysta_forluster.md §Hål 1: applyDivinePunishment
// and applyDivineBlessing used to run their outcome-mutating UPDATE through
// h.pool.Exec and throw the result away — a HARD-rule violation (CLAUDE.md
// §Events: "events store outcomes, not intentions" —
// {"type":"chariot_loss","amount":3}, never a bare roll-pending marker) — AND
// never called NotifyPlayer at all, so the loss/gain was invisible to the
// owner. Both are fixed via RETURNING-based capture of the real DB delta.
// These tests lock: the DivinePunishment/DivineBlessing event payload's
// "amount" is the actual mutation, "type" is unchanged, and exactly one
// notification reaches the owner carrying that same number.

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// divineNotifyRecorder is a minimal Broadcaster test double — unlike
// notifyRecorder (starvation_warning_test.go) it keeps the full payload, not
// just the tier, since these tests need to read "amount" back out of it.
type divineNotifyRecorder struct {
	mu    sync.Mutex
	calls []divineNotifyCall
}

type divineNotifyCall struct {
	playerID uuid.UUID
	kind     string
	level    int
	payload  map[string]any
}

func (r *divineNotifyRecorder) NotifyPlayer(ctx context.Context, worldID, playerID uuid.UUID, kind string, level int, payload any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	m, _ := payload.(map[string]any)
	r.calls = append(r.calls, divineNotifyCall{playerID: playerID, kind: kind, level: level, payload: m})
	return nil
}

// seedDivineGarrison (re)seeds one garrison unit of every type divine
// punishment/blessing can touch — war_chariot, spearman, galley — each sized
// well above the ~20%/min-2/min-1 deltas involved, so whichever of the four
// punishment branches or three blessing branches the roll picks always finds
// a non-zero real outcome to report. Clears any previous garrison first so it
// can be called again between loop iterations on the same settlement.
func seedDivineGarrison(t *testing.T, pool *pgxpool.Pool, ctx context.Context, worldID, settlementID, ownerID uuid.UUID) {
	t.Helper()
	if _, err := pool.Exec(ctx, `DELETE FROM units WHERE settlement_id = $1`, settlementID); err != nil {
		t.Fatalf("clear garrison: %v", err)
	}
	for _, u := range []struct {
		utype    string
		category string
		size     int
		crew     int
	}{
		{"war_chariot", "land", 50, 0},
		{"spearman", "land", 50, 0},
		{"galley", "naval", 1, 20},
	} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, settlement_id)
			 VALUES ($1, $2, $3, $4, $5, $6, 'garrison', $7)`,
			worldID, ownerID, u.utype, u.category, u.size, u.crew, settlementID,
		); err != nil {
			t.Fatalf("seed %s garrison: %v", u.utype, err)
		}
	}
	// grainAmount(400) < starvationWarningFixture's fixed cap(1000): headroom
	// for harvest_blessing's ×1.25 to actually gain something, not clip at cap.
	// rate=0 keeps settled() == amount regardless of when in the test the tick
	// worker's current_world_tick() is read — no starvation-trend confound.
	if _, err := pool.Exec(ctx,
		`UPDATE settlement_goods SET amount = 400, rate = 0, calc_tick = 0
		 WHERE settlement_id = $1 AND good_key = 'grain'`, settlementID,
	); err != nil {
		t.Fatalf("reseed grain: %v", err)
	}
}

// latestEventPayload reads back the most recently appended event of the given
// type for a stream (settlement) — the same table h.store.Append writes to.
func latestEventPayload(t *testing.T, pool *pgxpool.Pool, ctx context.Context, streamID uuid.UUID, eventType string) (typ string, amount float64) {
	t.Helper()
	var raw []byte
	if err := pool.QueryRow(ctx,
		`SELECT payload FROM events WHERE stream_id = $1 AND event_type = $2 ORDER BY id DESC LIMIT 1`,
		streamID, eventType,
	).Scan(&raw); err != nil {
		t.Fatalf("read %s event: %v", eventType, err)
	}
	var p struct {
		Type   string  `json:"type"`
		Amount float64 `json:"amount"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatalf("unmarshal %s payload: %v", eventType, err)
	}
	return p.Type, p.Amount
}

func TestApplyDivinePunishment_EventAndNotificationCarryRealAmount(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	worldID, settlementID, ownerID := starvationWarningFixture(t, 400, 0)

	// Loop: the punishment is picked by rand.Intn(4) inside the handler, so a
	// single call only exercises one of the four branches. seedDivineGarrison
	// gives every branch a non-zero outcome to find, so every iteration must
	// pass regardless of which one fired — this is the same "loop over the
	// random choice" pattern as economy's trade_internal_no_loss_test.go.
	const n = 16
	for i := 0; i < n; i++ {
		seedDivineGarrison(t, pool, ctx, worldID, settlementID, ownerID)

		rec := &divineNotifyRecorder{}
		h := NewTickHandler(pool, events.NewScheduler(pool, nil), events.NewStore(pool), rec)
		h.applyDivinePunishment(ctx, settlementID, worldID, ownerID)

		typ, amount := latestEventPayload(t, pool, ctx, settlementID, "DivinePunishment")
		if typ == "" {
			t.Fatalf("iter %d: event payload lost \"type\" — must stay unchanged, never reinterpreted", i)
		}
		if amount <= 0 {
			t.Errorf("iter %d: type=%s amount=%v, want > 0 (the actual RETURNING delta)", i, typ, amount)
		}

		if len(rec.calls) != 1 {
			t.Fatalf("iter %d: NotifyPlayer called %d times, want 1 (Fel A: it used to never be called)", i, len(rec.calls))
		}
		call := rec.calls[0]
		if call.playerID != ownerID {
			t.Errorf("iter %d: notified %s, want owner %s", i, call.playerID, ownerID)
		}
		if call.kind != "DivinePunishment" {
			t.Errorf("iter %d: notification kind = %q, want DivinePunishment", i, call.kind)
		}
		notifiedType, _ := call.payload["type"].(string)
		notifiedAmount, _ := call.payload["amount"].(float64)
		if notifiedType != typ || notifiedAmount != amount {
			t.Errorf("iter %d: notification (type=%s amount=%v) != event (type=%s amount=%v) — client must see the same truth as the server",
				i, notifiedType, notifiedAmount, typ, amount)
		}
	}
}

func TestApplyDivineBlessing_EventAndNotificationCarryRealAmount(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	worldID, settlementID, ownerID := starvationWarningFixture(t, 400, 0)

	const n = 16
	for i := 0; i < n; i++ {
		seedDivineGarrison(t, pool, ctx, worldID, settlementID, ownerID)

		rec := &divineNotifyRecorder{}
		h := NewTickHandler(pool, events.NewScheduler(pool, nil), events.NewStore(pool), rec)
		h.applyDivineBlessing(ctx, settlementID, worldID, ownerID)

		typ, amount := latestEventPayload(t, pool, ctx, settlementID, "DivineBlessing")
		if typ == "" {
			t.Fatalf("iter %d: event payload lost \"type\"", i)
		}
		if amount <= 0 {
			t.Errorf("iter %d: type=%s amount=%v, want > 0", i, typ, amount)
		}

		if len(rec.calls) != 1 {
			t.Fatalf("iter %d: NotifyPlayer called %d times, want 1 (mirror of applyDivinePunishment's Fel A)", i, len(rec.calls))
		}
		call := rec.calls[0]
		if call.kind != "DivineBlessing" {
			t.Errorf("iter %d: notification kind = %q, want DivineBlessing", i, call.kind)
		}
		notifiedAmount, _ := call.payload["amount"].(float64)
		if notifiedAmount != amount {
			t.Errorf("iter %d: notification amount %v != event amount %v", i, notifiedAmount, amount)
		}
	}
}
