package handlers

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
)

func TestNotificationDayBelongsToRecordedWorldAndNeverDrifts(t *testing.T) {
	f := setupNotifFixture(t)
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `UPDATE worlds SET current_tick=7 WHERE id=$1`, f.worldID); err != nil {
		t.Fatal(err)
	}
	f.archive(t, "BuildComplete")
	// The current world may move after a report was archived; its day must not.
	if _, err := f.pool.Exec(ctx, `UPDATE worlds SET current_tick=80 WHERE id=$1`, f.worldID); err != nil {
		t.Fatal(err)
	}
	code, out := f.do(t, "GET", "/worlds/"+f.worldID.String()+"/notifications")
	if code != 200 {
		t.Fatalf("HTTP%d: %v", code, out)
	}
	rows := out["notifications"].([]any)
	if len(rows) != 1 || rows[0].(map[string]any)["world_tick"] != float64(7) {
		t.Fatalf("notification must retain originating world day7 after world advances: %v", out)
	}
	// An unrelated world's tick cannot be borrowed (current_world_tick is global).
	var other uuid.UUID
	if err := f.pool.QueryRow(ctx, `INSERT INTO worlds(name,status,current_tick) VALUES('foreign-notification-day','archived',123) RETURNING id`).Scan(&other); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.pool.Exec(ctx, `DELETE FROM worlds WHERE id=$1`, other) })
	if _, err := f.pool.Exec(ctx, `INSERT INTO notifications(world_id,player_id,kind,level) VALUES($1,$2,'BuildComplete',3)`, other, f.playerID); err != nil {
		t.Fatal(err)
	}
	var recorded int
	if err := f.pool.QueryRow(ctx, `SELECT world_tick FROM notifications WHERE world_id=$1`, other).Scan(&recorded); err != nil || recorded != 123 {
		t.Fatalf("foreign world's captured day=%d err=%v", recorded, err)
	}
}

func TestNotificationDayMigrationKeepsUnknownHistoryUnknown(t *testing.T) {
	f := setupNotifFixture(t)
	ctx := context.Background()
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	down, err := os.ReadFile("../../db/migrations/187_notification_world_tick.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, string(down)); err != nil {
		t.Fatal(err)
	}
	bodies := []string{`{}`, `{"world_tick":0}`, `{"world_tick":17}`, `{"arrival_tick":99}`, `{"world_tick":"bad"}`, `{"world_tick":999999999999999}`, `{"world_tick":-1}`}
	ids := make([]uuid.UUID, len(bodies))
	for i, body := range bodies {
		if err := tx.QueryRow(ctx, `INSERT INTO notifications(world_id,player_id,kind,level,body_json) VALUES($1,$2,'Historic',3,$3) RETURNING id`, f.worldID, f.playerID, body).Scan(&ids[i]); err != nil {
			t.Fatal(err)
		}
	}
	snapshot := func() string {
		t.Helper()
		var s string
		if err := tx.QueryRow(ctx, `SELECT jsonb_agg(to_jsonb(n)-'world_tick' ORDER BY id)::text FROM notifications n WHERE world_id=$1`, f.worldID).Scan(&s); err != nil {
			t.Fatal(err)
		}
		return s
	}
	before := snapshot()
	up, err := os.ReadFile("../../db/migrations/187_notification_world_tick.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, string(up)); err != nil {
		t.Fatal(err)
	}
	if before != snapshot() {
		t.Fatal("migration rewrote historical payload/identity/time")
	}
	for i, id := range ids {
		var tick *int
		if err := tx.QueryRow(ctx, `SELECT world_tick FROM notifications WHERE id=$1`, id).Scan(&tick); err != nil {
			t.Fatal(err)
		}
		switch i {
		case 1:
			if tick == nil || *tick != 0 {
				t.Fatalf("explicit day0 lost: %v", tick)
			}
		case 2:
			if tick == nil || *tick != 17 {
				t.Fatalf("explicit day17 lost: %v", tick)
			}
		default:
			if tick != nil {
				t.Fatalf("invented historical day for%s: %d", bodies[i], *tick)
			}
		}
	}
}
