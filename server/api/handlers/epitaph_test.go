package handlers

// The epitaph crawl tells a fallen Wanax's whole reign (megaron_sista_staden.md;
// Timothy 2026-10-10). It must tell how the last city fell — collapse,
// annexation (SettlementCaptured) or burning (SettlementBurned) — even after a
// long reign whose daily bookkeeping (UpkeepSettled) fills the stream; and it
// must tell every city the Wanax founded, not only the last one, without the
// conqueror's later works. DB integration test (real Postgres, DATABASE_URL).

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
)

type epitaphFixture struct {
	t      *testing.T
	h      *WebHandler
	world  uuid.UUID
	player uuid.UUID
}

func newEpitaphFixture(t *testing.T) epitaphFixture {
	pool := worldIDTestPool(t)
	ctx := context.Background()
	f := epitaphFixture{t: t, h: &WebHandler{pool: pool}}
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status) VALUES ($1, 'archived') RETURNING id`,
		"epitaph-test-"+uuid.New().String(),
	).Scan(&f.world); err != nil {
		t.Fatalf("create world: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM worlds WHERE id = $1`, f.world) })
	f.player = f.mkPlayer("Agamemnon")
	return f
}

func (f epitaphFixture) mkPlayer(name string) uuid.UUID {
	var id uuid.UUID
	if err := f.h.pool.QueryRow(context.Background(),
		`INSERT INTO players (username, password_hash, wanax_name) VALUES ($1, 'x', $2) RETURNING id`,
		"epitaph-"+uuid.New().String(), name+"-"+uuid.New().String()[:4],
	).Scan(&id); err != nil {
		f.t.Fatalf("create player %s: %v", name, err)
	}
	return id
}

func (f epitaphFixture) name(player uuid.UUID) string {
	var n string
	_ = f.h.pool.QueryRow(context.Background(), `SELECT wanax_name FROM players WHERE id = $1`, player).Scan(&n)
	return n
}

// mkCity creates a settlement founded by founder at tick; parent marks a colony.
func (f epitaphFixture) mkCity(name string, founder uuid.UUID, q, tick int, parent *uuid.UUID) uuid.UUID {
	ctx := context.Background()
	var prov, id uuid.UUID
	if err := f.h.pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r) VALUES ($1, $2, 0) RETURNING id`, f.world, q,
	).Scan(&prov); err != nil {
		f.t.Fatalf("create province for %s: %v", name, err)
	}
	if err := f.h.pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, founder_id, founded_tick, founded_from)
		 VALUES ($1, $2, $3, 'minoan', $4, $5, $6) RETURNING id`,
		f.world, prov, name+"-"+uuid.New().String()[:4], founder, tick, parent,
	).Scan(&id); err != nil {
		f.t.Fatalf("create settlement %s: %v", name, err)
	}
	return id
}

func (f epitaphFixture) cityName(id uuid.UUID) string {
	var n string
	_ = f.h.pool.QueryRow(context.Background(), `SELECT name FROM settlements WHERE id = $1`, id).Scan(&n)
	return n
}

func (f epitaphFixture) event(stream uuid.UUID, tick int, eventType, payload string) {
	f.t.Helper()
	if _, err := f.h.pool.Exec(context.Background(),
		`INSERT INTO events (stream_id, stream_type, event_type, payload, world_id, world_tick)
		 VALUES ($1, 'province', $2, $3::jsonb, $4, $5)`,
		stream, eventType, payload, f.world, tick,
	); err != nil {
		f.t.Fatalf("append %s: %v", eventType, err)
	}
}

func TestEpitaphLines_TellsTheFallAfterALongReign(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name, eventType, payload string
		want                     func(rival, city string) string
	}{
		{"annexed", "SettlementCaptured", `{"new_owner":"RIVAL"}`, func(r, c string) string { return "Wanax " + r + " took " + c + "." }},
		{"burned", "SettlementBurned", `{"raider_id":"RIVAL"}`, func(r, c string) string { return "Wanax " + r + " burned " + c + "." }},
		{"collapsed", "CityCollapsed", `{"cause":"starvation"}`, func(_, c string) string { return "Hunger came. " + c + " fell." }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newEpitaphFixture(t)
			rival := f.mkPlayer("Nestor")
			city := f.mkCity("Tiryns", f.player, 0, 1, nil)
			f.event(city, 2, "BuildComplete", `{"building_type":"farm"}`)
			for i := 0; i < 250; i++ { // a long reign's daily bookkeeping
				f.event(city, 3+i, "UpkeepSettled", `{}`)
			}
			f.event(city, 260, c.eventType, strings.Replace(c.payload, "RIVAL", rival.String(), 1))

			got := strings.Join(f.h.epitaphLines(ctx, f.player, f.world, &city), "\n")
			name := f.cityName(city)
			if !strings.Contains(got, "Raised farm in "+name+".") {
				t.Errorf("crawl lost the reign's first deed:\n%s", got)
			}
			if want := c.want(f.name(rival), name); !strings.Contains(got, want) {
				t.Errorf("crawl does not tell the fall %q:\n%s", want, got)
			}
		})
	}
}

// TestEpitaphLines_TellsEveryCityTheWanaxFounded: the metropolis falls first,
// a colony carries on and falls last. The crawl must tell both cities in
// order, the battle in between, and stop Mycenae's story at its capture —
// Nestor's later farm there is Nestor's, not this Wanax's.
func TestEpitaphLines_TellsEveryCityTheWanaxFounded(t *testing.T) {
	ctx := context.Background()
	f := newEpitaphFixture(t)
	rival := f.mkPlayer("Nestor")
	mycenae := f.mkCity("Mycenae", f.player, 0, 1, nil)
	tiryns := f.mkCity("Tiryns", f.player, 5, 10, &mycenae)
	my, ti, ne := f.cityName(mycenae), f.cityName(tiryns), f.name(rival)

	f.event(mycenae, 3, "BuildComplete", `{"building_type":"temple"}`)
	f.event(tiryns, 12, "TrainComplete", `{"unit_type":"spearman"}`)

	// A battle at Mycenae, which this Wanax's defenders lose.
	var battle uuid.UUID
	if err := f.h.pool.QueryRow(ctx,
		`INSERT INTO battles (world_id, q, r, started_tick, current_tick, seed) VALUES ($1, 0, 0, 19, 20, 1) RETURNING id`, f.world,
	).Scan(&battle); err != nil {
		t.Fatalf("create battle: %v", err)
	}
	var unitID uuid.UUID
	if err := f.h.pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status) VALUES ($1, $2, 'spearman', 'land', 100, 0, 'positioned') RETURNING id`,
		f.world, f.player,
	).Scan(&unitID); err != nil {
		t.Fatalf("create unit: %v", err)
	}
	if _, err := f.h.pool.Exec(ctx,
		`INSERT INTO battle_participants (battle_id, unit_id, owner_id, side, joined_tick, initial_size, current_size)
		 VALUES ($1, $2, $3, 'defender', 19, 100, 0)`, battle, unitID, f.player); err != nil {
		t.Fatalf("join battle: %v", err)
	}
	f.event(battle, 20, "BattleEnded", `{"q":0,"r":0,"winner":"attacker"}`)

	f.event(mycenae, 21, "SettlementCaptured", `{"new_owner":"`+rival.String()+`"}`)
	f.event(mycenae, 30, "BuildComplete", `{"building_type":"farm"}`) // Nestor's work
	f.event(tiryns, 40, "CityCollapsed", `{"cause":"starvation"}`)

	got := f.h.epitaphLines(ctx, f.player, f.world, &tiryns)
	want := []string{
		my + " rose on the shore of the Thalassa.",
		"Raised temple in " + my + ".",
		"Founded " + ti + ".",
		"Mustered spearman from " + ti + ".",
		"Lost a battle at " + my + ".",
		"Wanax " + ne + " took " + my + ".",
		"Hunger came. " + ti + " fell.",
		"So ended a Wanax's reign.",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("crawl =\n%s\n\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}
