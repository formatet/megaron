package handlers

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"net/http"
	"strings"
	"testing"
)

func wholeUnitBody(ids ...uuid.UUID) string {
	b, _ := json.Marshal(map[string]any{"unit_ids": ids})
	return string(b)
}
func assertWholeUnit(t *testing.T, f *disbandFixture, id uuid.UUID, wantStatus string, wantSize int) {
	t.Helper()
	var status string
	var size int
	if err := f.pool.QueryRow(context.Background(), `SELECT status,size FROM units WHERE id=$1`, id).Scan(&status, &size); err != nil {
		t.Fatal(err)
	}
	if status != wantStatus || size != wantSize {
		t.Fatalf("unit %s: %s/%d want %s/%d", id, status, size, wantStatus, wantSize)
	}
}
func TestDisbandSelectedWholeUnitAndDamagedSurvivors(t *testing.T) {
	f := setupDisbandFixture(t, 1000)
	if _, err := f.pool.Exec(context.Background(), `INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) VALUES($1,'silver',77,0,1000,0)`, f.settlementID); err != nil {
		t.Fatal(err)
	}
	full := f.seedUnit(t, "spearman", "land", 100, 0)
	hurt := f.seedUnit(t, "spearman", "land", 41, 0)
	rec, resp := f.disband(t, wholeUnitBody(full))
	if rec.Code != 200 || resp["pop_restored"] != float64(100) {
		t.Fatalf("whole cohort: %d %s", rec.Code, rec.Body.String())
	}
	assertWholeUnit(t, f, full, "disbanded", 100)
	assertWholeUnit(t, f, hurt, "garrison", 41)
	rec, resp = f.disband(t, wholeUnitBody(hurt))
	if rec.Code != 200 || resp["pop_restored"] != float64(41) || f.population(t) != 1141 {
		t.Fatalf("survivors: %d %s", rec.Code, rec.Body.String())
	}
	assertWholeUnit(t, f, hurt, "disbanded", 41)
	var silver float64
	if err := f.pool.QueryRow(context.Background(), `SELECT amount FROM settlement_goods WHERE settlement_id=$1 AND good_key='silver'`, f.settlementID).Scan(&silver); err != nil || silver != 77 {
		t.Fatalf("silver changed: %v %v", silver, err)
	}
	rec, _ = f.disband(t, wholeUnitBody(hurt))
	if rec.Code != 422 || f.population(t) != 1141 {
		t.Fatalf("double refund: %d", rec.Code)
	}
}
func TestDisbandRejectsLegacyQuantityAndDuplicateWithoutChange(t *testing.T) {
	f := setupDisbandFixture(t, 1000)
	id := f.seedUnit(t, "spearman", "land", 100, 0)
	for _, body := range []string{`{"spearman":20}`, `{"ship":1}`, `{}`, `{"unit_ids":[]}`, `{"unit_ids":["bad"]}`, wholeUnitBody(id, id), `{"unit_ids":["` + id.String() + `"],"spearman":20}`} {
		rec, _ := f.disband(t, body)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: %d %s", body, rec.Code, rec.Body.String())
		}
		assertWholeUnit(t, f, id, "garrison", 100)
		if f.population(t) != 1000 {
			t.Fatal("legacy request changed population")
		}
		if strings.Contains(body, "spearman") && !strings.Contains(rec.Body.String(), "disband a whole unit by its id") {
			t.Fatal(rec.Body.String())
		}
	}
}
func TestDisbandAtomicOwnershipAndPresenceChecks(t *testing.T) {
	for _, tc := range []struct {
		change string
		code   int
	}{
		{`UPDATE units SET owner_id=(SELECT id FROM players WHERE id<>units.owner_id LIMIT 1) WHERE id=$1`, 403},
		{`UPDATE units SET status='marching' WHERE id=$1`, 422},
		{`UPDATE units SET status='training' WHERE id=$1`, 422},
		{`UPDATE units SET settlement_id=NULL WHERE id=$1`, 422},
		{`UPDATE units SET type='nomadic_host' WHERE id=$1`, 422},
	} {
		t.Run(tc.change, func(t *testing.T) {
			f := setupDisbandFixture(t, 1000)
			a := f.seedUnit(t, "spearman", "land", 100, 0)
			b := f.seedUnit(t, "spearman", "land", 61, 0)
			// Ensure an alternate owner exists without relying on another test's rows.
			_, err := f.pool.Exec(context.Background(), `INSERT INTO players (id,username,password_hash) VALUES ($1,$2,'x')`, uuid.New(), "foreign-"+uuid.New().String())
			if err != nil {
				t.Fatal(err)
			}
			if _, err = f.pool.Exec(context.Background(), tc.change, b); err != nil {
				t.Fatal(err)
			}
			rec, _ := f.disband(t, wholeUnitBody(a, b))
			if rec.Code != tc.code {
				t.Fatalf("got %d: %s", rec.Code, rec.Body.String())
			}
			assertWholeUnit(t, f, a, "garrison", 100)
			if f.population(t) != 1000 {
				t.Fatal("failed batch returned people")
			}
		})
	}
}
