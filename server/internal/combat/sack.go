package combat

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/gossip"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/tick"
	"formatet/megaron/server/internal/transport"
)

// sackSettlement is the "sack" conquest choice (Del 2b, Timothy 2026-07-10): instead
// of annexing the settlement, the attacker loots it and razes it to a ruin. Called
// from both victory paths (applyAttackerWins, resolveAmphibiousAssault) in place of
// the annex branch. The caller is responsible for placing the attacker's own unit
// (positioned, not garrisoned — sackSettlement does not touch the attacker's unit
// row at all) before calling this.
//
// Loot formula (decision-locked): silver 50%; every other good 0.5/goods.weight —
// lighter goods travel better (luxury/purple w1 → 50%, silver/grain/copper/tin w2 →
// 25%, stone/cedar/timber w3 → 17%, horses w5 → 10%). The loot is dispatched home as
// a physical, interceptable transport (kind="plunder") — not credited directly —
// so a sentry can seize it in transit like any other caravan.
func (h *UnitArrivalHandler) sackSettlement(
	ctx context.Context, tx pgx.Tx,
	attackerOwnerID uuid.UUID, dest destSettlement, destQ, destR int, worldID uuid.UUID,
) error {
	settlementID := *dest.settlementID

	// ── Loot manifest: silver 50%, everything else weighted by portability. ──
	// `g.weight > 0` is load-bearing, not cosmetic: weight-0 goods (cult — temple
	// output, not a physical good you can cart home) would make the `0.5 / g.weight`
	// divisor a division-by-zero that aborts the whole tx mid-stream. Excluding them
	// is also correct on the merits — cult is not lootable plunder.
	rows, err := tx.Query(ctx,
		`SELECT sg.good_key, floor(settled(sg.amount, sg.rate, sg.calc_tick) *
		        CASE WHEN sg.good_key = 'silver' THEN 0.5 ELSE 0.5 / g.weight END) AS loot
		 FROM settlement_goods sg JOIN goods g ON g.key = sg.good_key
		 WHERE sg.settlement_id = $1 AND g.weight > 0`,
		settlementID,
	)
	if err != nil {
		return fmt.Errorf("sack: load loot manifest: %w", err)
	}
	type lootRow struct {
		good string
		qty  float64
	}
	var loots []lootRow
	for rows.Next() {
		var lr lootRow
		if scanErr := rows.Scan(&lr.good, &lr.qty); scanErr != nil {
			rows.Close()
			return fmt.Errorf("sack: scan loot row: %w", scanErr)
		}
		if lr.qty > 0 {
			loots = append(loots, lr)
		}
	}
	rows.Close()

	manifest := transport.Manifest{}
	for _, lr := range loots {
		manifest[lr.good] += lr.qty
		if _, err := tx.Exec(ctx,
			`UPDATE settlement_goods SET
			   amount    = GREATEST(0, settled(amount, rate, calc_tick) - $2),
			   calc_tick = current_world_tick()
			 WHERE settlement_id = $1 AND good_key = $3`,
			settlementID, lr.qty, lr.good,
		); err != nil {
			return fmt.Errorf("sack: deduct looted %s: %w", lr.good, err)
		}
	}

	// Sitos fund silver is a separate pot (mig 072, not a settlement_goods row) —
	// same 50% cut, folded into the manifest's silver line.
	var fundSilver float64
	if err := tx.QueryRow(ctx,
		`SELECT floor(GREATEST(0, sitos_fund_silver) * 0.5) FROM settlements WHERE id = $1`,
		settlementID,
	).Scan(&fundSilver); err != nil {
		return fmt.Errorf("sack: load sitos fund: %w", err)
	}
	if fundSilver > 0 {
		manifest["silver"] += fundSilver
		if _, err := tx.Exec(ctx,
			`UPDATE settlements SET sitos_fund_silver = sitos_fund_silver - $2 WHERE id = $1`,
			settlementID, fundSilver,
		); err != nil {
			return fmt.Errorf("sack: deduct sitos fund: %w", err)
		}
	}

	// ── Dispatch the plunder caravan toward the attacker's capital. ──
	if len(manifest) > 0 {
		var capitalID uuid.UUID
		var capQ, capR int
		if err := tx.QueryRow(ctx,
			`SELECT s.id, p.map_q, p.map_r FROM settlements s JOIN provinces p ON p.id = s.province_id
			 WHERE s.owner_id = $1 AND s.world_id = $2 AND s.is_capital = true`,
			attackerOwnerID, worldID,
		).Scan(&capitalID, &capQ, &capR); err != nil {
			// No capital to send the loot home to (should not happen — the attacker
			// just marched from somewhere — but never fail the sack over it).
			slog.Warn("sack: attacker has no capital, loot lost", "owner", attackerOwnerID, "err", err)
		} else {
			_, pathHours, pathOK, pathErr := province.FindPath(ctx, tx, worldID,
				province.MapPosition{Q: destQ, R: destR},
				province.MapPosition{Q: capQ, R: capR},
				"land",
			)
			var moveHours float64
			if pathErr == nil && pathOK {
				moveHours = pathHours
			} else {
				// Island-raid degradation (decision-locked, accepted for MVP): a sacked
				// island has no land route home, so the caravan cannot be positioned
				// mid-transit (InterpolatePosition needs a walkable path) and so cannot
				// be intercepted — but it still delivers safely on arrival.
				dist := province.HexDistance(
					province.MapPosition{Q: destQ, R: destR},
					province.MapPosition{Q: capQ, R: capR},
				)
				if dist < 1 {
					dist = 1
				}
				moveHours = province.TerrainMoveHours(dest.terrain) * float64(dist)
			}
			travelTicks := int(math.Round(moveHours))
			if travelTicks < 1 {
				travelTicks = 1
			}
			var currentTick int
			_ = tx.QueryRow(ctx, `SELECT current_world_tick()`).Scan(&currentTick)
			now := h.clk.Now()
			arrivesAt := now.Add(time.Duration(travelTicks*tick.TickSeconds) * time.Second)

			if _, err := transport.Dispatch(ctx, tx, h.scheduler, transport.DispatchParams{
				WorldID:       worldID,
				OwnerID:       attackerOwnerID,
				Kind:          "plunder",
				OriginID:      settlementID,
				DestID:        capitalID,
				Category:      "land",
				OriginQ:       destQ,
				OriginR:       destR,
				DestQ:         capQ,
				DestR:         capR,
				DepartsAt:     now,
				ArrivesAt:     arrivesAt,
				DueTick:       currentTick + travelTicks,
				Manifest:      manifest,
				Interceptable: true,
			}); err != nil {
				return fmt.Errorf("sack: dispatch plunder caravan: %w", err)
			}
		}
	}

	// ── Raze the settlement: ownerless ruin, not an occupied colony. ──
	var fallenName string
	_ = tx.QueryRow(ctx, `SELECT name FROM settlements WHERE id = $1`, settlementID).Scan(&fallenName)
	if _, err := tx.Exec(ctx,
		`UPDATE settlements SET
		   owner_id     = NULL,
		   state        = 'razed',
		   control_type = 'occupied',
		   kingdom_id   = NULL,
		   updated_at   = now()
		 WHERE id = $1`,
		settlementID,
	); err != nil {
		return fmt.Errorf("sack: raze settlement: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`UPDATE provinces SET territory_state = 'free', controller_id = NULL WHERE id = $1`,
		dest.provinceID,
	); err != nil {
		return fmt.Errorf("sack: free province: %w", err)
	}

	// Garrison dies with the city — nothing to hand over, nothing left to defend a ruin.
	if _, err := tx.Exec(ctx,
		`UPDATE units SET status = 'disbanded', size = 0, updated_at = now()
		 WHERE settlement_id = $1 AND status = 'garrison'`,
		settlementID,
	); err != nil {
		return fmt.Errorf("sack: disband garrison: %w", err)
	}

	// Succession / game-over for the sacked owner — ownership already cleared above.
	if dest.ownerID != nil {
		if _, err := handleOwnerCityLoss(ctx, tx, *dest.ownerID, worldID, settlementID); err != nil {
			return fmt.Errorf("sack: handle owner city loss: %w", err)
		}
	}

	// Rumor: distinct from conquest — a sack is remembered differently than an
	// occupation (temenos_gossip.md PASS 2b).
	if fallenName != "" {
		if err := gossip.Broadcast(ctx, tx, worldID, settlementID, "military",
			fallenName+" was sacked and razed.", 6,
			gossip.ImportanceMajor, settlementID, ""); err != nil {
			slog.Warn("sackSettlement: broadcast gossip", "settlement", settlementID, "err", err)
		}
	}

	_, _ = h.eventStore.Append(ctx, settlementID, events.StreamProvince, "SettlementSacked",
		map[string]any{
			"settlement_id": settlementID,
			"former_owner":  dest.ownerID,
			"raider":        attackerOwnerID,
			"looted":        manifest,
		}, worldID, nil)

	if h.hub != nil {
		if dest.ownerID != nil {
			_ = h.hub.NotifyPlayer(ctx, worldID, *dest.ownerID, "SettlementSacked", 2, map[string]any{
				"settlement_id": settlementID, "role": "defender",
			})
		}
		_ = h.hub.NotifyPlayer(ctx, worldID, attackerOwnerID, "SettlementSacked", 3, map[string]any{
			"settlement_id": settlementID, "role": "attacker", "looted": manifest,
		})
	}

	slog.Info("settlement sacked and razed", "settlement", settlementID, "name", fallenName,
		"raider", attackerOwnerID, "former_owner", dest.ownerID, "looted", manifest)

	return nil
}
