package combat

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Cancel one selected preparation read: either the tick read or the second
// terrain snapshot (A* has already succeeded). The public StartMarch uses a
// real pool; all other queries reach PostgreSQL normally.
type failMarchPreparationRead struct {
	reads  int
	query  string
	failAt int
}

func (f *failMarchPreparationRead) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if strings.TrimSpace(data.SQL) == f.query {
		f.reads++
		if f.reads == f.failAt {
			queryCtx, cancel := context.WithCancel(ctx)
			cancel()
			return queryCtx
		}
	}
	return ctx
}

func (*failMarchPreparationRead) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func TestStartMarch_RoutePricingFailureLeavesStateUnchanged(t *testing.T) {
	testMarchPreparationFailure(t, "SELECT q, r, terrain FROM map_tiles WHERE world_id = $1", 2)
}

func TestStartMarch_TickReadFailureLeavesStateUnchanged(t *testing.T) {
	testMarchPreparationFailure(t, "SELECT current_world_tick()", 1)
}

func testMarchPreparationFailure(t *testing.T, query string, failAt int) {
	t.Helper()
	pool, worldID, ownerID, settlementID := setupLandMarchWorldWithHome(t)
	ctx := context.Background()
	clk := clock.NewTestClock(time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC))

	// Keep the colony destination outside the home's radius-2 catchment.
	for q := 3; q <= 6; q++ {
		if _, err := pool.Exec(ctx, `INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, $2, 0, 'plains')`, worldID, q); err != nil {
			t.Fatal(err)
		}
	}
	var unitID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, status, q, r, support_settlement_id)
		 VALUES ($1, $2, 'spearman', 'land', 100, 'positioned', 0, 0, $3) RETURNING id`,
		worldID, ownerID, settlementID).Scan(&unitID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, calc_tick)
		 VALUES ($1, 'silver', 500, 0, current_world_tick())`, settlementID); err != nil {
		t.Fatal(err)
	}
	// Snapshot the complete rows, not just status: NULL route/arrival fields,
	// purse and the silver lazy tuple must all remain exactly unchanged.
	var unitBefore, goodsBefore string
	if err := pool.QueryRow(ctx, `SELECT row_to_json(u)::text FROM units u WHERE id = $1`, unitID).Scan(&unitBefore); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT row_to_json(g)::text FROM settlement_goods g WHERE settlement_id = $1 AND good_key = 'silver'`, settlementID).Scan(&goodsBefore); err != nil {
		t.Fatal(err)
	}
	var jobsBefore, eventsBefore int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM scheduled_events WHERE world_id = $1`, worldID).Scan(&jobsBefore); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM events WHERE world_id = $1`, worldID).Scan(&eventsBefore); err != nil {
		t.Fatal(err)
	}

	fault := &failMarchPreparationRead{query: query, failAt: failAt}
	cfg := pool.Config()
	cfg.ConnConfig.Tracer = fault
	faultPool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(faultPool.Close)
	result, err := StartMarch(ctx, faultPool, events.NewScheduler(faultPool, clk), events.NewStore(faultPool), clk,
		MarchOrder{WorldID: worldID, PlayerID: ownerID, UnitID: unitID, TargetQ: 6, TargetR: 0, Intent: "colonize", Name: "Route integrity"}, nil)
	if fault.reads != failAt {
		t.Fatalf("fault hit %d matching reads, want %d", fault.reads, failAt)
	}
	var rejected *OrderReject
	if result != nil || !errors.As(err, &rejected) || rejected.Status != http.StatusInternalServerError {
		t.Errorf("march preparation failure returned %v, %v; want internal-error rejection", result, err)
	}
	var unitAfter, goodsAfter string
	if err := pool.QueryRow(ctx, `SELECT row_to_json(u)::text FROM units u WHERE id = $1`, unitID).Scan(&unitAfter); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT row_to_json(g)::text FROM settlement_goods g WHERE settlement_id = $1 AND good_key = 'silver'`, settlementID).Scan(&goodsAfter); err != nil {
		t.Fatal(err)
	}
	if unitAfter != unitBefore {
		t.Errorf("failed route preparation changed unit: before=%s after=%s", unitBefore, unitAfter)
	}
	if goodsAfter != goodsBefore {
		t.Errorf("failed route preparation charged settlement: before=%s after=%s", goodsBefore, goodsAfter)
	}
	var jobsAfter, eventsAfter int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM scheduled_events WHERE world_id = $1`, worldID).Scan(&jobsAfter); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM events WHERE world_id = $1`, worldID).Scan(&eventsAfter); err != nil {
		t.Fatal(err)
	}
	if jobsAfter != jobsBefore || eventsAfter != eventsBefore {
		t.Errorf("failed route preparation created durable work: jobs %d->%d, events %d->%d", jobsBefore, jobsAfter, eventsBefore, eventsAfter)
	}
}
