package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"testing"

	"github.com/google/uuid"
)

// Real authenticated Recruit calls: ten land training jobs must leave the
// shipyard queue empty, in both the early and transaction-locked checks.
func TestRecruitShipQueueIgnoresTenLandTrainingJobs(t *testing.T) {
	f := setupRecruitShipFixture(t)
	for i := 0; i < 10; i++ {
		rec, _ := f.post(t, recruitPath(f), map[string]any{"unit_type": "spearman"})
		if rec.Code != http.StatusCreated {
			t.Fatalf("land recruit %d: %d %s", i, rec.Code, rec.Body.String())
		}
	}
	var jobs int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM scheduled_events WHERE world_id=$1 AND event_type='TrainComplete' AND processed_at IS NULL AND failed_at IS NULL`, f.worldID).Scan(&jobs); err != nil {
		t.Fatal(err)
	}
	if jobs != 10 {
		t.Fatalf("fixture has %d land jobs, want10", jobs)
	}
	rec, _ := f.post(t, recruitPath(f), map[string]any{"unit_type": "galley"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("ship after ten land jobs: %d %s; want201", rec.Code, rec.Body.String())
	}
}

func TestRecruitShipQueueCapsTenNavalJobs(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		name := "shipyard_work"
		if legacy {
			name = "legacy_TrainComplete"
		}
		t.Run(name, func(t *testing.T) {
			f := setupRecruitShipFixture(t)
			rec, body := f.post(t, recruitPath(f), map[string]any{"unit_type": "galley", "count": 10})
			if rec.Code != http.StatusCreated {
				t.Fatalf("first ten: %d %s", rec.Code, rec.Body.String())
			}
			if legacy {
				for _, id := range body["unit_ids"].([]any) {
					addLegacyNavalTimer(t, f, id.(string), true)
				}
			}
			before := snapshotRecruitState(t, f)
			rec, body = f.post(t, recruitPath(f), map[string]any{"unit_type": "galley"})
			if rec.Code != http.StatusUnprocessableEntity || body["error"] != "build queue would overflow: 10 pending + 1 new > 10" {
				t.Fatalf("eleventh: %d %s", rec.Code, rec.Body.String())
			}
			if after := snapshotRecruitState(t, f); !reflect.DeepEqual(before, after) {
				t.Fatalf("rejected eleventh changed state: before=%+v after=%+v", before, after)
			}
		})
	}
}

func TestRecruitShipQueueDoesNotDoubleCountConvertedNavalTimers(t *testing.T) {
	f := setupRecruitShipFixture(t)
	rec, body := f.post(t, recruitPath(f), map[string]any{"unit_type": "galley", "count": 9})
	if rec.Code != http.StatusCreated {
		t.Fatalf("first nine: %d %s", rec.Code, rec.Body.String())
	}
	// A leftover old timer for work already tracked by units is not a second job.
	for _, id := range body["unit_ids"].([]any) {
		addLegacyNavalTimer(t, f, id.(string), false)
	}
	rec, _ = f.post(t, recruitPath(f), map[string]any{"unit_type": "galley"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("tenth with stale timers: %d %s; want201", rec.Code, rec.Body.String())
	}
	rec, _ = f.post(t, recruitPath(f), map[string]any{"unit_type": "galley"})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("eleventh with stale timers: %d %s; want422", rec.Code, rec.Body.String())
	}
}

func addLegacyNavalTimer(t *testing.T, f *recruitShipFixture, id string, clearWork bool) {
	t.Helper()
	ctx := context.Background()
	if clearWork {
		// A real pre-conversion ship had a saved deadline. Retire this
		// synthetic legacy state afterward so deploy-transition tests do not
		// inherit jobs belonging to a different test's historical scenario.
		t.Cleanup(func() {
			if _, err := f.pool.Exec(ctx, `UPDATE units SET status='garrison',build_complete_at=NULL WHERE id=$1`, id); err != nil {
				t.Error(err)
			}
		})
		if _, err := f.pool.Exec(ctx, `UPDATE units SET shipyard_work_remaining=NULL,build_complete_at=now()+interval '1 day' WHERE id=$1`, id); err != nil {
			t.Fatal(err)
		}
	}
	payload, err := json.Marshal(map[string]any{"unit_id": id, "settlement_id": f.settlementID, "unit_type": "galley", "count": 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO scheduled_events(world_id,event_type,payload,process_after,due_tick) VALUES($1,'TrainComplete',$2,now(),100)`, f.worldID, payload); err != nil {
		t.Fatal(err)
	}
}

// Orphaned timers and finished/failed jobs are not pending ship construction.
func TestRecruitShipQueueIgnoresInactiveLegacyTimers(t *testing.T) {
	for _, state := range []string{"orphan", "processed", "failed"} {
		t.Run(state, func(t *testing.T) {
			f := setupRecruitShipFixture(t)
			if state == "orphan" {
				for i := 0; i < 10; i++ {
					addLegacyNavalTimer(t, f, uuid.NewString(), false)
				}
			} else {
				rec, body := f.post(t, recruitPath(f), map[string]any{"unit_type": "galley", "count": 10})
				if rec.Code != http.StatusCreated {
					t.Fatalf("setup ships: %d %s", rec.Code, rec.Body.String())
				}
				for _, id := range body["unit_ids"].([]any) {
					addLegacyNavalTimer(t, f, id.(string), true)
				}
				column := "processed_at"
				if state == "failed" {
					column = "failed_at"
				}
				if _, err := f.pool.Exec(context.Background(), "UPDATE scheduled_events SET "+column+"=now() WHERE world_id=$1 AND event_type='TrainComplete'", f.worldID); err != nil {
					t.Fatal(err)
				}
			}
			rec, _ := f.post(t, recruitPath(f), map[string]any{"unit_type": "galley", "count": 10})
			if rec.Code != http.StatusCreated {
				t.Fatalf("ten ships with %s timers: %d %s; want201", state, rec.Code, rec.Body.String())
			}
		})
	}
}
