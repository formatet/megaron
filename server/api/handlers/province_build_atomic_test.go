package handlers

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// A build is paid and queued in one transaction: if the queue row cannot be
// written, the city keeps its timber and stone. Until 2026-10-24 the costs
// were committed first, so a failed INSERT left a paid build with no job.
func TestBuild_FailedQueueKeepsTheCost(t *testing.T) {
	ctx := context.Background()
	f := buildHexFixture(t)
	stock := func() (timber, stone float64) {
		t.Helper()
		if err := f.pool.QueryRow(ctx,
			`SELECT (SELECT amount FROM settlement_goods WHERE settlement_id=$1 AND good_key='timber'),
			        (SELECT amount FROM settlement_goods WHERE settlement_id=$1 AND good_key='stone')`,
			f.settlementID).Scan(&timber, &stone); err != nil {
			t.Fatal(err)
		}
		return
	}
	timber0, stone0 := stock()

	fn := "fail_build_queue_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := f.pool.Exec(ctx, `CREATE FUNCTION `+fn+`() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
	  IF NEW.world_id='`+f.worldID.String()+`' THEN RAISE EXCEPTION 'build_queue failure'; END IF; RETURN NEW; END $$`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `CREATE TRIGGER `+fn+` BEFORE INSERT ON build_queue FOR EACH ROW EXECUTE FUNCTION `+fn+`()`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = f.pool.Exec(ctx, `DROP TRIGGER IF EXISTS `+fn+` ON build_queue`)
		_, _ = f.pool.Exec(ctx, `DROP FUNCTION IF EXISTS `+fn+`()`)
	})

	code, resp := f.do(t, http.MethodPost, f.buildPath(), map[string]any{"building_type": "farm", "hex_q": 1, "hex_r": 0})
	if code != http.StatusInternalServerError {
		t.Fatalf("build with a failing queue = %d: %v, want 500", code, resp)
	}
	if timber, stone := stock(); timber != timber0 || stone != stone0 {
		t.Fatalf("queue failed but the cost was taken: timber %v→%v, stone %v→%v", timber0, timber, stone0, stone)
	}
	var jobs int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM scheduled_events WHERE world_id=$1 AND event_type='BuildComplete'`, f.worldID).Scan(&jobs); err != nil {
		t.Fatal(err)
	}
	if jobs != 0 {
		t.Fatalf("%d BuildComplete jobs scheduled for a build that was never queued", jobs)
	}
}
