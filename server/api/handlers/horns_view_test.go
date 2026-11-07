package handlers

// What the OWNER sees of Asterion's two blessings (böner steg 5b): a troop under the horns carries
// "the bull's horns, N game days" in its blessings; a caravan under the Thread carries "the Thread,
// N game days" in the trades list. Another Wanax sees neither.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"formatet/megaron/server/internal/religion"
)

func TestHornsView_TheOwnersTroopsShowTheHornsWhileTheyLast(t *testing.T) {
	f := newShipRepairFixture(t, true, map[string]float64{"timber": 1000})
	f.router.Get("/worlds/{worldID}/units", f.uh.ListUnits)
	f.exec(t, `UPDATE worlds SET current_tick=100 WHERE id=$1`, f.worldID)
	var covered, bare string
	for _, dst := range []*string{&covered, &bare} {
		if err := f.pool.QueryRow(context.Background(), `INSERT INTO units (world_id, owner_id, type, category, size, crew, status, settlement_id, support_settlement_id)
			VALUES ($1,$2,'spearman','land',50,0,'garrison',$3,$3) RETURNING id`, f.worldID, f.playerID, f.settlementID).Scan(dst); err != nil {
			t.Fatal(err)
		}
	}
	f.exec(t, `INSERT INTO prayer_blessings(world_id,owner_id,effect,scope,unit_id,from_tick,until_tick) VALUES($1,$2,$3,'unit',$4,100,112)`,
		f.worldID, f.playerID, religion.EffectBlessedHorns, covered)
	req := httptest.NewRequest(http.MethodGet, "/worlds/"+f.worldID.String()+"/units", nil)
	req.Header.Set("Authorization", "Bearer "+f.accessToken)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Units []struct {
			ID        string   `json:"id"`
			Blessings []string `json:"blessings"`
		} `json:"units"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	seen := 0
	for _, u := range resp.Units {
		switch u.ID {
		case covered:
			seen++
			if len(u.Blessings) != 1 || u.Blessings[0] != "the bull's horns, 12 game days" {
				t.Fatalf("covered troop blessings %v", u.Blessings)
			}
		case bare:
			seen++
			if len(u.Blessings) != 0 {
				t.Fatalf("an uncovered troop shows %v", u.Blessings)
			}
		}
	}
	if seen != 2 {
		t.Fatalf("saw %d of the two troops: %s", seen, rec.Body.String())
	}
	// Expired: nothing to show.
	f.exec(t, `UPDATE worlds SET current_tick=112 WHERE id=$1`, f.worldID)
	rec = httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	if strings.Contains(rec.Body.String(), "bull's horns") {
		t.Fatalf("an expired blessing is still shown: %s", rec.Body.String())
	}
}
