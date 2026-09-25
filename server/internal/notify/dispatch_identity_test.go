package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// A dispatch and its Notifications archive row are the SAME object to the
// client (megaron_plan_dispatches.md §1, "ett fönster, två dörrar"). Before
// this, the WS frame carried only kind+payload, so the chip had no way to say
// WHICH archive row it was — it could not mark that row read when dismissed,
// and the strip could not be rebuilt from the archive without duplicating
// every chip on the next push. These tests hold the two fields that make the
// identity real: the id, and the level the archive already stored.

// TestNotifyPlayerCarriesLevelWithoutPool holds the half that needs no DB: even
// with no archive at all, the push tells the client how urgent the event is.
// The archive rows have been level-styled since long before this; the strip
// could not be, because the level never left the server.
func TestNotifyPlayerCarriesLevelWithoutPool(t *testing.T) {
	h := New() // no pool: nothing is archived, so there is no id to carry
	worldID, playerID := uuid.New(), uuid.New()
	conn, closer := dialHub(t, h, worldID, playerID)
	defer closer()

	if !waitFor(t, 2*time.Second, func() bool { return clientCount(h) == 1 }) {
		t.Fatal("client never registered")
	}
	if err := h.NotifyPlayer(context.Background(), worldID, playerID, "FoodShortfall", 2,
		map[string]any{"unmet": 312}); err != nil {
		t.Fatalf("NotifyPlayer: %v", err)
	}

	msg := readMsg(t, conn)
	if msg.Kind != "FoodShortfall" {
		t.Errorf("kind = %q, want FoodShortfall", msg.Kind)
	}
	if msg.Level != 2 {
		t.Errorf("level = %d, want 2 — the client cannot weight the strip without it", msg.Level)
	}
	if msg.ID != "" {
		t.Errorf("id = %q, want empty with no pool — nothing was archived", msg.ID)
	}
}

// TestNotifyPlayerCarriesArchiveID is the other half, and needs a real DB: the
// id on the wire must be the id of the row that was actually inserted, not a
// fresh one. Mutation to try: drop `RETURNING id` (back to Exec) and this fails
// on an empty id — which is exactly the state a dismissed chip could not
// recover from.
func TestNotifyPlayerCarriesArchiveID(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set — skipping DB integration test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	defer pool.Close()

	var worldID, playerID uuid.UUID
	// status 'archived', not the 'active' default: a partial unique index
	// (one_active_world) permits exactly one active world, so taking the default
	// makes this test pass or fail on whether some OTHER test in the same
	// database happens to hold the active slot.
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status, map_width, map_height) VALUES ($1, 'archived', 12, 12) RETURNING id`,
		fmt.Sprintf("notify-identity-%s", uuid.NewString()[:8]),
	).Scan(&worldID); err != nil {
		t.Fatalf("world: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, password_hash) VALUES ($1, 'x') RETURNING id`,
		fmt.Sprintf("notify-identity-%s", uuid.NewString()[:8]),
	).Scan(&playerID); err != nil {
		t.Fatalf("player: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM worlds WHERE id = $1`, worldID) })

	h := New()
	h.SetPool(pool)
	conn, closer := dialHub(t, h, worldID, playerID)
	defer closer()
	if !waitFor(t, 2*time.Second, func() bool { return clientCount(h) == 1 }) {
		t.Fatal("client never registered")
	}

	if err := h.NotifyPlayer(ctx, worldID, playerID, "SiegeStarted", 2,
		map[string]any{"name": "Phaistos"}); err != nil {
		t.Fatalf("NotifyPlayer: %v", err)
	}
	msg := readMsg(t, conn)

	var archivedID uuid.UUID
	if err := pool.QueryRow(ctx,
		`SELECT id FROM notifications WHERE world_id = $1 AND player_id = $2`,
		worldID, playerID,
	).Scan(&archivedID); err != nil {
		t.Fatalf("archived row: %v", err)
	}
	if msg.ID != archivedID.String() {
		t.Errorf("pushed id = %q, archived id = %q — the chip would mark the wrong row read",
			msg.ID, archivedID)
	}
	if msg.Level != 2 {
		t.Errorf("level = %d, want 2", msg.Level)
	}
}

// readMsg reads one non-heartbeat frame off conn.
func readMsg(t *testing.T, conn interface {
	ReadMessage() (int, []byte, error)
	SetReadDeadline(time.Time) error
}) Msg {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		var m Msg
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatalf("unmarshal %s: %v", raw, err)
		}
		if m.Kind == "Heartbeat" {
			continue
		}
		return m
	}
}
