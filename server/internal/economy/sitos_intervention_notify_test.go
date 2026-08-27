package economy

// The granary eating into its reserve is the one Sitos moment a Wanax must
// hear about. Storing is routine and stays silent (as the fund's "buy" leg
// did); releasing is the city living off what it set aside.
//
// It uses a kind of its OWN — not the fund's "SitosIntervention", which fired
// constantly and is on the noise filters in cmd_notifications.go and notif.js.
// Reusing it would have hidden the rare event behind a filter built for the
// frequent one.

import (
	"context"
	"testing"

	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
)

type fakeSitosBroadcaster struct {
	notified []string
}

func (f *fakeSitosBroadcaster) BroadcastEvent(worldID uuid.UUID, kind string, payload any) {}

func (f *fakeSitosBroadcaster) NotifyPlayer(ctx context.Context, worldID, playerID uuid.UUID, kind string, level int, payload any) error {
	f.notified = append(f.notified, kind)
	return nil
}

func TestGranary_NotifiesOwnerOnRelease(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	cfg := testSitosCfg()

	const tick = 100
	// 1000 → 10, 20000 → 200 (mig 136, ÷100 — same food-unit scale as DailyFoodNeed;
	// pop=1000 now needs 5/day, so this must stay under the LowDays=10 threshold (50)).
	worldID, settlementID := granaryFixture(t, pool, ctx, tick, 1000,
		[]fixtureGood{{"grain", 10, 1000000}}, map[string]float64{"grain": 200})

	fb := &fakeSitosBroadcaster{}
	h := NewSitosTickHandler(pool, events.NewScheduler(pool, nil), events.NewStore(pool), fb, cfg)
	if err := h.tickSettlement(ctx, settlementID, worldID, 1); err != nil {
		t.Fatalf("tickSettlement: %v", err)
	}

	found := false
	for _, k := range fb.notified {
		if k == NotifySitosGranaryRelease {
			found = true
		}
	}
	if !found {
		t.Errorf("NotifyPlayer calls = %v, want %q among them", fb.notified, NotifySitosGranaryRelease)
	}
}

// TestGranary_StoringIsSilent: the routine leg must not notify, or the feed
// fills with it every tick and buries the release — precisely what happened to
// the fund's SitosIntervention (~99 % of the notification feed).
func TestGranary_StoringIsSilent(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	cfg := testSitosCfg()

	const tick = 100
	worldID, settlementID := granaryFixture(t, pool, ctx, tick, 1000,
		[]fixtureGood{{"grain", 20000, 1000000}}, nil)

	fb := &fakeSitosBroadcaster{}
	h := NewSitosTickHandler(pool, events.NewScheduler(pool, nil), events.NewStore(pool), fb, cfg)
	if err := h.tickSettlement(ctx, settlementID, worldID, 1); err != nil {
		t.Fatalf("tickSettlement: %v", err)
	}
	if len(fb.notified) != 0 {
		t.Errorf("storing notified %v, want nothing", fb.notified)
	}
}
