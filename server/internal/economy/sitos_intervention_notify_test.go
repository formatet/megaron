package economy

// Regression test for Fas 2c: the Sitos fund's safety net (selling emergency
// stock into a shortage) previously only showed up in `ticklog` — nothing
// notified the owner when it actually happened, so a Wanax had to think to
// go looking for it after the fact. This verifies the "sell" (rescue) leg
// notifies the owner; "buy" (routine surplus absorption) intentionally does
// not (see the comment in stabilizeGood).

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

func TestSitosTick_NotifiesOwnerOnRescueSell(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	cfg := testSitosCfg()

	const tick = 100
	// Same deep-shortage fixture as TestSitosTick_SilverConserved — forces
	// action.Kind == "sell" (the rescue case).
	worldID, settlementID := sitosFixture(t, pool, ctx, tick, 1000 /*fund*/, 5000 /*silver*/, 2000 /*grain*/, 5 /*rate*/, 0)

	fb := &fakeSitosBroadcaster{}
	h := NewSitosTickHandler(pool, events.NewScheduler(pool, nil), events.NewStore(pool), fb, cfg)
	grainBase, err := GoodBaseValue(ctx, pool, "grain")
	if err != nil {
		t.Fatalf("grain base value: %v", err)
	}
	if err := h.tickSettlement(ctx, settlementID, worldID, grainBase); err != nil {
		t.Fatalf("tickSettlement: %v", err)
	}

	found := false
	for _, k := range fb.notified {
		if k == "SitosIntervention" {
			found = true
		}
	}
	if !found {
		t.Errorf("NotifyPlayer calls = %v, want a \"SitosIntervention\" among them (rescue-sell must notify the owner)", fb.notified)
	}
}
