package main

// The reconciliation loop deactivates an erased player's Agora account
// (megaron_plan_kontoradering.md): an outage keeps the flag for the next pass,
// success clears it, and an erased player is never provisioned. DB test.

import (
	"context"
	"testing"

	"formatet/megaron/server/internal/agora"
	"github.com/google/uuid"
)

func (f *fakeAgoraRemote) Deactivate(_ context.Context, _, localpart string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.down {
		return agora.ErrUnavailable
	}
	f.deactivated = append(f.deactivated, localpart)
	return nil
}

func TestAgoraDeactivatesErasedPlayerAfterOutage(t *testing.T) {
	f := newAgoraFixture(t, "Nestor-"+uuid.NewString(), 4)
	r := newFakeAgoraRemote()
	s := &agoraService{pool: f.pool, remote: r, room: "!test"}
	ctx := context.Background()
	if err := s.reconcile(ctx, f.player); err != nil {
		t.Fatal(err)
	}
	var localpart string
	if err := f.pool.QueryRow(ctx, `SELECT agora_localpart FROM players WHERE id=$1`, f.player).Scan(&localpart); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx,
		`UPDATE players SET erased_at = now(), agora_deactivate_pending = true WHERE id = $1`, f.player); err != nil {
		t.Fatal(err)
	}
	pending := func() bool {
		var p bool
		if err := f.pool.QueryRow(ctx, `SELECT agora_deactivate_pending FROM players WHERE id=$1`, f.player).Scan(&p); err != nil {
			t.Fatal(err)
		}
		return p
	}

	r.down = true
	s.deactivateErased(ctx)
	count := func() int {
		n := 0
		for _, lp := range r.deactivated {
			if lp == localpart {
				n++
			}
		}
		return n
	}
	if !pending() || count() != 0 {
		t.Fatalf("outage: pending=%v deactivated=%v, want the flag kept for the next pass", pending(), r.deactivated)
	}
	r.down = false
	s.deactivateErased(ctx)
	if pending() || count() != 1 {
		t.Fatalf("after outage: pending=%v deactivated=%v, want %q deactivated once and the flag cleared", pending(), r.deactivated, localpart)
	}
	s.deactivateErased(ctx)
	if count() != 1 {
		t.Fatalf("a cleared flag deactivated again: %v", r.deactivated)
	}
}

func TestAgoraNeverProvisionsErasedPlayer(t *testing.T) {
	f := newAgoraFixture(t, "Erased-"+uuid.NewString(), 4)
	r := newFakeAgoraRemote()
	s := &agoraService{pool: f.pool, remote: r, room: "!test"}
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `UPDATE players SET erased_at = now() WHERE id = $1`, f.player); err != nil {
		t.Fatal(err)
	}
	s.pass(ctx)
	if r.createCalls != 0 {
		t.Fatalf("pass called Create %d times for an erased player, want 0", r.createCalls)
	}
}
