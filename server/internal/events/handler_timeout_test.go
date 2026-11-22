package events

import (
	"context"
	"testing"
	"time"
)

// The per-world tick handlers walk every city in one run; at a full world the
// 5 s default is not enough (measured 2026-11-22). They must get their own
// timeout, and everything else must keep the default.
func TestWorker_TimeoutForRegisteredHandler(t *testing.T) {
	w := NewWorker(nil, nil)
	noop := func(context.Context, ScheduledEvent) error { return nil }
	w.Register("Plain", noop)
	w.RegisterWithTimeout("WorldWide", noop, WorldTickHandlerTimeout)

	if got := w.timeoutFor("Plain"); got != DefaultHandlerTimeout {
		t.Errorf("Plain timeout = %v, want %v", got, DefaultHandlerTimeout)
	}
	if got := w.timeoutFor("WorldWide"); got != WorldTickHandlerTimeout {
		t.Errorf("WorldWide timeout = %v, want %v", got, WorldTickHandlerTimeout)
	}
	if WorldTickHandlerTimeout <= DefaultHandlerTimeout {
		t.Fatalf("WorldTickHandlerTimeout %v must exceed the default %v", WorldTickHandlerTimeout, DefaultHandlerTimeout)
	}
	if got := w.timeoutFor("Unregistered"); got != DefaultHandlerTimeout {
		t.Errorf("unregistered timeout = %v, want %v", got, DefaultHandlerTimeout)
	}
}

// dispatch must hand the handler a context with ITS timeout, not the default.
func TestWorker_DispatchUsesRegisteredTimeout(t *testing.T) {
	pool := testWorkerPool(t)
	w := NewWorker(pool, nil)
	var remaining time.Duration
	w.RegisterWithTimeout("WorldWide", func(ctx context.Context, _ ScheduledEvent) error {
		dl, ok := ctx.Deadline()
		if !ok {
			t.Fatal("handler context has no deadline")
		}
		remaining = time.Until(dl)
		return nil
	}, WorldTickHandlerTimeout)
	w.dispatch(context.Background(), ScheduledEvent{ID: -1, EventType: "WorldWide"})
	if remaining <= DefaultHandlerTimeout {
		t.Fatalf("handler deadline %v away, want more than the %v default", remaining, DefaultHandlerTimeout)
	}
}
