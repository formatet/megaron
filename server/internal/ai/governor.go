// Package ai implements passive AI players and AI governors for Poleia.
package ai

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PassiveGovernorTick runs the daily logic for AI-governed settlements.
// A passive governor keeps the settlement stable — pays maintenance, does nothing
// aggressive. Future: intermediate governors may recruit or manage loyalty.
func PassiveGovernorTick(ctx context.Context, pool *pgxpool.Pool, settlementID, worldID uuid.UUID) error {
	// Verify settlement is AI-governed.
	var governorIsAI bool
	err := pool.QueryRow(ctx,
		`SELECT governor_is_ai FROM settlements WHERE id = $1`,
		settlementID,
	).Scan(&governorIsAI)
	if err != nil {
		return fmt.Errorf("load settlement: %w", err)
	}
	if !governorIsAI {
		return nil
	}

	// Passive governor: ensure loyalty doesn't fall below 2 by consuming a small
	// silver reserve as a care action. If silver > 20, register a minimal gift event.
	var silver float64
	if err = pool.QueryRow(ctx,
		`SELECT COALESCE(settled(amount, rate, calc_tick), 0)
		 FROM settlement_goods WHERE settlement_id = $1 AND good_key = 'silver'`,
		settlementID,
	).Scan(&silver); err != nil {
		silver = 0
	}

	var loyalty int
	if err = pool.QueryRow(ctx,
		`SELECT loyalty FROM settlements WHERE id = $1`,
		settlementID,
	).Scan(&loyalty); err != nil {
		return fmt.Errorf("load settlement resources: %w", err)
	}

	if silver >= 20 && loyalty <= 2 {
		_, err = pool.Exec(ctx,
			`UPDATE settlement_goods
			   SET amount  = settled(amount, rate, calc_tick) - 10,
			       calc_tick = current_world_tick()
			 WHERE settlement_id = $1 AND good_key = 'silver'
			   AND settled(amount, rate, calc_tick) >= 10`,
			settlementID,
		)
		if err != nil {
			slog.Warn("passive governor silver deduction failed", "settlement", settlementID)
		}
	}

	return nil
}

// VoteWeighting returns an AI member's vote bias for a candidate in an election.
// Bias favours candidates with high kharis. Range: [0.0, 1.0].
func VoteWeighting(candidateKharis int, localPantheonAlignment float64) float64 {
	base := 0.5
	switch {
	case candidateKharis >= 800:
		base += 0.3 * localPantheonAlignment
	case candidateKharis >= 400:
		base += 0.1 * localPantheonAlignment
	case candidateKharis < 100:
		base -= 0.35
	case candidateKharis < 200:
		base -= 0.2
	}
	if base < 0 {
		return 0
	}
	if base > 1 {
		return 1
	}
	return base
}

// DivinInterventionProbability returns the probability that the gods override an
// election result for a candidate with the given kharis level.
// Formula from thalassa_kingdoms.md (vault): P = (400 - kharis) / 400 × 0.30
func DivineInterventionProbability(kharis int) float64 {
	if kharis >= 400 {
		return 0
	}
	p := float64(400-kharis) / 400.0 * 0.30
	if p < 0 {
		return 0
	}
	return p
}
