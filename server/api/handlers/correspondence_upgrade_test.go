package handlers

import (
	"context"
	"os"
	"testing"
)

func TestCorrespondenceUpgradeRetainsCompletedAcceptanceWithoutInventingTime(t *testing.T) {
	f := caseFixture(t)
	ctx := context.Background()
	ids := map[string]string{}
	for _, status := range []string{"accepted", "returned", "declined", "expired", "cancelled"} {
		letter, _ := sendCase(t, f, "Historic "+status, "")
		ids[status] = letter
		if _, err := f.pool.Exec(ctx, `UPDATE messengers SET trade_offer=jsonb_build_object('kind','buy','want_good','tin','want_qty',2,'offer_silver',5,'status',$2::text) WHERE id=$1`, letter, status); err != nil {
			t.Fatal(err)
		}
	}
	down, err := os.ReadFile("../../db/migrations/209_correspondence_memory.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	up, err := os.ReadFile("../../db/migrations/209_correspondence_memory.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	// Recreate retired private tables before rehearsing the historical 208->209 transition.
	retired, err := os.ReadFile("../../db/migrations/210_brevladan.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, string(retired)); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, string(down)); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, string(up)); err != nil {
		t.Fatal(err)
	}
	for status, id := range ids {
		var recorded, noInventedTime bool
		if err = tx.QueryRow(ctx, `SELECT accepted_xid IS NOT NULL,accepted_at IS NULL FROM correspondence_letters WHERE id=$1`, id).Scan(&recorded, &noInventedTime); err != nil {
			t.Fatal(err)
		}
		if recorded != (status == "accepted" || status == "returned") || !noInventedTime {
			t.Fatalf("historical %s: acceptance=%v unknown_time=%v", status, recorded, noInventedTime)
		}
	}
}
