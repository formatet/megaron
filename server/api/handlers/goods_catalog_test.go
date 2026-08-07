package handlers

// Test for GoodsHandler.TradeableCatalogue (GET /api/v1/goods) —
// megaron_plan_offertens_varulista.md. The web offer form used to take a
// good key as free text on four fields; a stray letter produced a dead
// offer with escrowed silver locked for OfferExpiryTicks (7 in-game days)
// before this slice existed at all. This test's job is narrower than that
// history: prove that the NEW read surface (this file) and the EXISTING
// validator (MessengerHandler.tradeableGood, messenger.go) agree on exactly
// the same set of goods — the actual invariant the plan names ("Offertens
// valbara varor och offertens serverkontroll är samma mängd, alltid").
//
// TestGoodsCatalogue_MatchesValidator below deliberately does NOT compare
// the catalogue's output against a second, independently-typed copy of the
// same SQL string — that would test only that a string literal was pasted
// twice (the exact trap 2026-07-30's compress-tests fell into: "ett test
// som speglar produktionskonfigurationen testar ingenting"). Instead it
// drives MessengerHandler.tradeableGood itself, the real validator Send()
// calls, for every key the catalogue returned, and for the keys the plan
// requires excluded (silver, cult, and a key that cannot exist) — i.e. it
// exercises the actual decision path, not a mirror of it.
import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGoodsCatalogue_MatchesValidator(t *testing.T) {
	pool := p10TestPool(t)
	ctx := context.Background()

	gh := NewGoodsHandler(pool)
	mh := &MessengerHandler{pool: pool}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/goods", nil)
	rec := httptest.NewRecorder()
	gh.TradeableCatalogue(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("TradeableCatalogue = %d: %s", rec.Code, rec.Body.String())
	}

	var got []TradeableGood
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("parse TradeableCatalogue response: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("TradeableCatalogue returned zero goods — expected at least the seeded commodities")
	}

	// Anchor to the DB directly, independent of both handlers: the catalogue's
	// row count and key set must equal `goods` minus silver/cult minus any
	// parked good (mig 114 — purple/pottery/horses are kept in the catalog/
	// lore but withdrawn from trade, Temenos_varutaxonomi_sol.md §4.2).
	rows, err := pool.Query(ctx, `SELECT key FROM goods WHERE key NOT IN ('silver', 'cult') AND status = 'active' ORDER BY key`)
	if err != nil {
		t.Fatalf("query goods: %v", err)
	}
	defer rows.Close()
	var wantKeys []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			t.Fatalf("scan good key: %v", err)
		}
		wantKeys = append(wantKeys, k)
	}
	if len(got) != len(wantKeys) {
		t.Fatalf("TradeableCatalogue returned %d goods, DB (excluding silver/cult) has %d", len(got), len(wantKeys))
	}
	for i, k := range wantKeys {
		if got[i].Key != k {
			t.Fatalf("catalogue order/content mismatch at index %d: got %q, want %q", i, got[i].Key, k)
		}
		if got[i].Name == "" {
			t.Errorf("good %q has empty display name", k)
		}
	}

	// silver and cult must be absent from the catalogue — they are currency
	// and temple labor respectively, never a tradeable good (see
	// MessengerHandler.tradeableGood's doc comment, messenger.go). purple,
	// pottery and horses are parked (mig 114): still real rows in `goods`,
	// but withdrawn from trade until their return conditions are met.
	for _, forbidden := range []string{"silver", "cult", "purple", "pottery", "horses"} {
		for _, g := range got {
			if g.Key == forbidden {
				t.Errorf("TradeableCatalogue must never list %q", forbidden)
			}
		}
	}

	// The real invariant: every key the catalogue lists must be accepted by
	// the real validator, and the two forbidden keys plus one that cannot
	// exist must be rejected by it. This drives MessengerHandler.tradeableGood
	// itself — not a second read of the same query — so a future change that
	// lets the two lists drift apart (e.g. someone hand-edits one query but
	// not the other) fails HERE, on the actual decision path Send() uses.
	for _, g := range got {
		if _, ok := mh.tradeableGood(ctx, g.Key); !ok {
			t.Errorf("tradeableGood(%q) = false, want true — catalogue lists it but the validator rejects it", g.Key)
		}
	}
	for _, forbidden := range []string{"silver", "cult", "purple", "pottery", "horses", "not-a-real-good-xyz"} {
		if _, ok := mh.tradeableGood(ctx, forbidden); ok {
			t.Errorf("tradeableGood(%q) = true, want false", forbidden)
		}
	}
}

// The catalogue's JSON shape must be simple enough for a client <select> to
// consume directly: a key it can send back verbatim as want_good/offer_good,
// and a name to show. Tier/Category ride along for free (already on the
// `goods` row, no extra query) but are not load-bearing for the dropdown.
func TestGoodsCatalogue_JSONShape(t *testing.T) {
	pool := p10TestPool(t)

	gh := NewGoodsHandler(pool)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/goods", nil)
	rec := httptest.NewRecorder()
	gh.TradeableCatalogue(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("TradeableCatalogue = %d: %s", rec.Code, rec.Body.String())
	}

	var got []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("parse response as generic JSON: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("expected at least one good")
	}
	for _, field := range []string{"key", "name", "tier", "category"} {
		if _, ok := got[0][field]; !ok {
			t.Errorf("goods[0] missing field %q: %+v", field, got[0])
		}
	}
}
