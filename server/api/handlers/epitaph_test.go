package handlers

// The epitaph crawl must tell how the last city fell (megaron_sista_staden.md).
// Three paths end a reign — collapse, annexation (SettlementCaptured) and
// burning (SettlementBurned) — and a long reign fills the capital's stream with
// daily bookkeeping (UpkeepSettled) that must not crowd the fall out of the
// crawl. DB integration test (real Postgres, gated by DATABASE_URL).

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestEpitaphLines_TellsTheFallAfterALongReign(t *testing.T) {
	pool := worldIDTestPool(t)
	ctx := context.Background()
	h := &WebHandler{pool: pool}

	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status) VALUES ($1, 'archived') RETURNING id`,
		"epitaph-test-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create world: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM worlds WHERE id = $1`, worldID) })

	var conqueror uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, password_hash, wanax_name) VALUES ($1, 'x', 'Nestor') RETURNING id`,
		"nestor-"+uuid.New().String(),
	).Scan(&conqueror); err != nil {
		t.Fatalf("create conqueror: %v", err)
	}

	appendEvent := func(stream uuid.UUID, eventType, payload string) {
		t.Helper()
		if _, err := pool.Exec(ctx,
			`INSERT INTO events (stream_id, stream_type, event_type, payload, world_id, world_tick)
			 VALUES ($1, 'province', $2, $3::jsonb, $4, 0)`,
			stream, eventType, payload, worldID,
		); err != nil {
			t.Fatalf("append %s: %v", eventType, err)
		}
	}

	cases := []struct {
		name, eventType, payload, want string
	}{
		{"annexed", "SettlementCaptured", `{"new_owner":"` + conqueror.String() + `"}`, "Wanax Nestor took Tiryns."},
		{"burned", "SettlementBurned", `{"raider_id":"` + conqueror.String() + `"}`, "Wanax Nestor burned Tiryns."},
		{"collapsed", "CityCollapsed", `{"cause":"starvation"}`, "Hunger came. Tiryns fell."},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			city := uuid.New()
			appendEvent(city, "BuildComplete", `{"building_type":"farm"}`)
			for i := 0; i < 250; i++ { // a long reign's daily bookkeeping
				appendEvent(city, "UpkeepSettled", `{}`)
			}
			appendEvent(city, c.eventType, c.payload)

			lines := h.epitaphLines(ctx, &city, "Tiryns")
			got := strings.Join(lines, "\n")
			if !strings.Contains(got, "Raised farm in Tiryns.") {
				t.Errorf("crawl lost the reign's first deed:\n%s", got)
			}
			if !strings.Contains(got, c.want) {
				t.Errorf("crawl does not tell the fall %q:\n%s", c.want, got)
			}
		})
	}
}
