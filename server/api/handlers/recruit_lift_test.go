package handlers

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func liftedEventCount(t *testing.T, f *recruitShipFixture) int {
	t.Helper()
	var n int
	if err := f.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM events WHERE stream_id = $1 AND event_type = 'PlacementsLiftedV1'`, f.settlementID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// A draft that takes a man who stood at a post lifts that post in the same
// transaction, records PlacementsLiftedV1 and tells the client in the response.
func TestRecruit_DraftLiftsPlacementAndReportsIt(t *testing.T) {
	f := setupRecruitShipFixture(t)
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `DELETE FROM settlement_placement WHERE settlement_id=$1`, f.settlementID); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 50; i++ { // pop 5000 -> 50 posts, all manned
		if _, err := f.pool.Exec(ctx,
			`INSERT INTO settlement_placement(settlement_id,gubbe_ordinal,target_kind,hex_q,hex_r,good_key) VALUES($1,$2,'hex',$3,200,'grain')`,
			f.settlementID, i, 200+i); err != nil {
			t.Fatal(err)
		}
	}
	path := "/worlds/" + f.worldID.String() + "/provinces/" + f.provinceID.String() + "/recruit"
	rec, resp := f.post(t, path, map[string]any{"unit_type": "spearman", "men": 100})
	if rec.Code != http.StatusCreated {
		t.Fatalf("recruit = %d %q", rec.Code, rec.Body.String())
	}
	lifted, _ := resp["workers_lifted"].([]any)
	if len(lifted) != 1 {
		t.Fatalf("workers_lifted = %v, want 1 entry", resp["workers_lifted"])
	}
	if txt, _ := resp["workers_lifted_text"].(string); !strings.Contains(txt, "men were drafted") {
		t.Errorf("workers_lifted_text = %q", txt)
	}
	var posts int
	_ = f.pool.QueryRow(ctx, `SELECT count(*) FROM settlement_placement WHERE settlement_id=$1`, f.settlementID).Scan(&posts)
	if posts != 49 {
		t.Errorf("placements = %d, want 49", posts)
	}
	if n := liftedEventCount(t, f); n != 1 {
		t.Errorf("PlacementsLiftedV1 events = %d, want 1", n)
	}
}

// A draft that takes only the free pool lifts nothing and announces nothing.
func TestRecruit_DraftWithinFreePoolSaysNothing(t *testing.T) {
	f := setupRecruitShipFixture(t)
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `DELETE FROM settlement_placement WHERE settlement_id=$1`, f.settlementID); err != nil {
		t.Fatal(err)
	}
	path := "/worlds/" + f.worldID.String() + "/provinces/" + f.provinceID.String() + "/recruit"
	rec, resp := f.post(t, path, map[string]any{"unit_type": "spearman", "men": 100})
	if rec.Code != http.StatusCreated {
		t.Fatalf("recruit = %d %q", rec.Code, rec.Body.String())
	}
	if _, has := resp["workers_lifted"]; has {
		t.Errorf("workers_lifted present: %v", resp["workers_lifted"])
	}
	if n := liftedEventCount(t, f); n != 0 {
		t.Errorf("PlacementsLiftedV1 events = %d, want 0", n)
	}
}
