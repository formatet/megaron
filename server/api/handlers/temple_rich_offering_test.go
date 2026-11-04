package handlers

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func TestSetRichOffering_PutDeleteAndRefusals(t *testing.T) {
	f := buildHexFixture(t)
	ctx := context.Background()
	var id uuid.UUID
	if err := f.pool.QueryRow(ctx, `INSERT INTO buildings(settlement_id,building_type,level,god_id) VALUES($1,'temple',1,'potnia') RETURNING id`, f.settlementID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	path := "/worlds/" + f.worldID.String() + "/temples/" + id.String() + "/rich-offering"
	stored := func() *string {
		var g *string
		if err := f.pool.QueryRow(ctx, `SELECT rich_offering_good FROM buildings WHERE id=$1`, id).Scan(&g); err != nil {
			t.Fatal(err)
		}
		return g
	}
	if code, body := f.do(t, http.MethodPut, path, map[string]any{"good": "fish"}); code != http.StatusOK {
		t.Fatalf("put: %d %v", code, body)
	}
	if g := stored(); g == nil || *g != "fish" {
		t.Fatalf("stored %v", g)
	}
	if code, _ := f.do(t, http.MethodPut, path, map[string]any{"good": "unobtainium"}); code != http.StatusBadRequest {
		t.Fatalf("unpriced good: %d", code)
	}
	if g := stored(); g == nil || *g != "fish" {
		t.Fatalf("refusal changed state: %v", g)
	}
	other := "/worlds/" + f.worldID.String() + "/temples/" + uuid.New().String() + "/rich-offering"
	if code, _ := f.do(t, http.MethodPut, other, map[string]any{"good": "fish"}); code != http.StatusNotFound {
		t.Fatalf("foreign temple: %d", code)
	}
	if code, body := f.do(t, http.MethodDelete, path, nil); code != http.StatusOK || stored() != nil {
		t.Fatalf("delete: %d %v %v", code, body, stored())
	}
}
