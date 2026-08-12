package economy

import (
	"context"
	"errors"
	"testing"
)

// TestIsShippableGood_Cult pins the regel's one home directly against the
// economy package, not just through the HTTP handlers that already exercise
// it indirectly (api/handlers/province_trade_cult_test.go,
// TestAutoAlloc_CultNotBudgeted). Cult reads weight=0 from the goods catalog
// (migration 055) and must therefore never be shippable — see
// IsShippableGood's own doc comment for why weight, not a hardcoded string,
// is the discriminator.
func TestIsShippableGood_Cult(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	weight, shippable, err := IsShippableGood(ctx, pool, GoodCult)
	if err != nil {
		t.Fatalf("IsShippableGood(cult): %v", err)
	}
	if shippable {
		t.Errorf("IsShippableGood(cult) shippable = true, want false (weight = %v)", weight)
	}
	if weight != 0 {
		t.Errorf("IsShippableGood(cult) weight = %v, want 0", weight)
	}
}

// TestIsShippableGood_OrdinaryGood is the counter-case: a real, physically
// carried good must come back shippable with its catalog weight intact.
func TestIsShippableGood_OrdinaryGood(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	weight, shippable, err := IsShippableGood(ctx, pool, GoodGrain)
	if err != nil {
		t.Fatalf("IsShippableGood(grain): %v", err)
	}
	if !shippable {
		t.Errorf("IsShippableGood(grain) shippable = false, want true")
	}
	if weight <= 0 {
		t.Errorf("IsShippableGood(grain) weight = %v, want > 0", weight)
	}
}

// TestIsShippableGood_UnknownGood pins ErrUnknownGood for a key that isn't in
// the catalog at all — the case api/handlers/province.go's Trade handler
// turns into a generic 400 "unknown good", distinct from cult's specific
// "temple labor" message (province_trade_cult_test.go's
// TestTrade_UnknownGoodStillRejected covers that HTTP-level distinction;
// this pins the underlying error the distinction is built on).
func TestIsShippableGood_UnknownGood(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	_, _, err := IsShippableGood(ctx, pool, "unobtainium")
	if !errors.Is(err, ErrUnknownGood) {
		t.Errorf("IsShippableGood(unobtainium) err = %v, want ErrUnknownGood", err)
	}
}
