package combat

import (
	"context"
	"fmt"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/unit"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"time"
)

// RecordRecruitReservation allocates the actual payment already made by Recruit,
// not a new quote from the catalogue. Legacy partial units remain unknown.
func RecordRecruitReservation(ctx context.Context, tx pgx.Tx, id uuid.UUID, paid map[string]float64, kharis float64, people, totalPeople int, fresh bool) error {
	var costs map[string]float64
	var heldKharis float64
	if err := tx.QueryRow(ctx, `SELECT recruitment_costs,recruitment_kharis FROM units WHERE id=$1 FOR UPDATE`, id).Scan(&costs, &heldKharis); err != nil {
		return err
	}
	if costs == nil {
		if !fresh {
			return nil
		}
		costs = map[string]float64{}
	}
	if people < 0 || totalPeople <= 0 || people > totalPeople {
		return fmt.Errorf("invalid recruitment allocation")
	}
	share := float64(people) / float64(totalPeople)
	for good, n := range paid {
		costs[good] += n * share
	}
	_, err := tx.Exec(ctx, `UPDATE units SET recruitment_costs=$2,recruitment_kharis=$3 WHERE id=$1`, id, costs, heldKharis+kharis*share)
	return err
}

type CancelTrainingResult struct {
	UnitID           uuid.UUID          `json:"unit_id"`
	PopRestored      int                `json:"pop_restored"`
	Refunded         map[string]float64 `json:"refunded"`
	KharisRefunded   float64            `json:"kharis_refunded"`
	AlreadyCancelled bool               `json:"already_cancelled"`
}

func CancelTraining(ctx context.Context, pool *pgxpool.Pool, store *events.Store, worldID, owner, id uuid.UUID, now time.Time) (*CancelTrainingResult, error) {
	var cityID *uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT settlement_id FROM units WHERE id=$1 AND world_id=$2 AND owner_id=$3`, id, worldID, owner).Scan(&cityID); err != nil {
		if err == pgx.ErrNoRows {
			return nil, reject(http.StatusNotFound, "unit not found")
		}
		return nil, err
	}
	if cityID == nil {
		return nil, reject(http.StatusConflict, "this unit is not training in a city")
	}
	city := *cityID
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	// Match Recruit's lock order: city first, unit second.
	var locked uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT id FROM settlements WHERE id=$1 AND world_id=$2 AND owner_id=$3 AND state='active' FOR UPDATE`, city, worldID, owner).Scan(&locked); err != nil {
		return nil, reject(http.StatusConflict, "the recruitment city is no longer yours")
	}
	var status, category string
	var size, crew int
	var costs map[string]float64
	var kharis float64
	var cancelled *time.Time
	if err := tx.QueryRow(ctx, `SELECT status,category,size,COALESCE(crew,0),recruitment_costs,recruitment_kharis,recruitment_cancelled_at FROM units WHERE id=$1 AND world_id=$2 AND owner_id=$3 AND settlement_id=$4 FOR UPDATE`, id, worldID, owner, city).Scan(&status, &category, &size, &crew, &costs, &kharis, &cancelled); err != nil {
		return nil, err
	}
	result := &CancelTrainingResult{UnitID: id, Refunded: map[string]float64{}}
	if cancelled != nil {
		result.AlreadyCancelled = true
		return result, nil
	}
	if status != "forming" && status != "training" {
		return nil, reject(http.StatusConflict, "training has already finished — use Disband to return a finished unit's people")
	}
	if costs == nil {
		return nil, reject(http.StatusConflict, "this older order has no recorded refundable reservation; once ready, it can be disbanded")
	}
	restored := size
	if category == "naval" {
		restored = crew
	}
	if _, err := tx.Exec(ctx, `UPDATE units SET status='disbanded',build_complete_at=NULL,recruitment_cancelled_at=$2,updated_at=now() WHERE id=$1`, id, now); err != nil {
		return nil, err
	}
	for good, n := range costs {
		if n < 0 {
			return nil, fmt.Errorf("negative recruitment reservation")
		}
		if n > 0 {
			if _, err := tx.Exec(ctx, `SELECT credit_settlement_good($1,$2,$3,'recruit_refund')`, city, good, n); err != nil {
				return nil, err
			}
			result.Refunded[good] = n
		}
	}
	if kharis > 0 {
		if _, err := tx.Exec(ctx, `UPDATE player_world_records SET kharis_amount=LEAST(kharis_cap,settled(kharis_amount,kharis_rate,kharis_calc_tick)+$3),kharis_calc_tick=current_world_tick() WHERE world_id=$1 AND player_id=$2`, worldID, owner, kharis); err != nil {
			return nil, err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE settlements SET population=population+$2 WHERE id=$1`, city, restored); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM scheduled_events WHERE world_id=$1 AND event_type='TrainComplete' AND payload->>'unit_id'=$2 AND processed_at IS NULL AND failed_at IS NULL`, worldID, id.String()); err != nil {
		return nil, err
	}
	if err := economy.RecomputeProduction(ctx, tx, city); err != nil {
		return nil, err
	}
	result.PopRestored = restored
	result.KharisRefunded = kharis
	var event *events.Event
	if store != nil {
		event, err = store.AppendTx(ctx, tx, id, events.StreamType(unit.StreamUnit), "RecruitmentCancelled", result, worldID, nil)
		if err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	if event != nil {
		store.RecordCommitted(ctx, event)
	}
	return result, nil
}
