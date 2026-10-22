package handlers

import (
	"context"
	"fmt"
	"strings"

	"formatet/megaron/server/internal/combat"
	"github.com/google/uuid"
)

// marchSilverForecast projects payroll only. Separate city treasuries are never
// pooled: a multi-city selection reports the shortest positive-drain runway.
func (h *UnitHandler) marchSilverForecast(ctx context.Context, worldID, ownerID, primary uuid.UUID, rawIDs string) (map[string]any, error) {
	selected := map[uuid.UUID]bool{primary: true}
	if rawIDs != "" {
		for _, raw := range strings.Split(rawIDs, ",") {
			id, err := uuid.Parse(raw)
			if err != nil {
				return nil, fmt.Errorf("invalid selected unit ID")
			}
			selected[id] = true
		}
	}
	type row struct {
		id                     uuid.UUID
		typ, category, status  string
		size                   int
		location, payer, cargo *uuid.UUID
	}
	rows, err := h.pool.Query(ctx, `SELECT u.id,u.type,u.category,u.status,u.size,u.settlement_id,
	 (SELECT s.id FROM settlements s WHERE s.id=u.support_settlement_id AND s.owner_id=u.owner_id AND s.world_id=u.world_id),u.cargo_unit_id
	 FROM units u WHERE u.world_id=$1 AND u.owner_id=$2`, worldID, ownerID)
	if err != nil {
		return nil, err
	}
	all := map[uuid.UUID]row{}
	for rows.Next() {
		var u row
		if err := rows.Scan(&u.id, &u.typ, &u.category, &u.status, &u.size, &u.location, &u.payer, &u.cargo); err != nil {
			rows.Close()
			return nil, err
		}
		all[u.id] = u
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	for id := range selected {
		if _, ok := all[id]; !ok {
			return nil, fmt.Errorf("not your selected unit in this world")
		}
	}
	// A loaded ship takes its embarked unit with it. Include the cargo exactly
	// once even when the caller also selected it explicitly.
	for id := range selected {
		if cargo := all[id].cargo; cargo != nil {
			if _, ok := all[*cargo]; ok {
				selected[*cargo] = true
			}
		}
	}
	result := map[string]any{"upkeep_silver_home": float64(0), "upkeep_silver_field": float64(0), "silver_runway_ticks": nil, "silver_payer_count": 0}
	var subsistence bool
	if err := h.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM founder_phase WHERE world_id=$1 AND owner_id=$2 AND active)`, worldID, ownerID).Scan(&subsistence); err != nil {
		return nil, err
	}
	if subsistence {
		return result, nil
	}
	share := combat.UpkeepSoldShare()
	home, field := 0.0, 0.0
	drain := map[uuid.UUID]float64{}
	payers := map[uuid.UUID]bool{}
	unfunded := false
	for id, u := range all {
		switch u.status {
		case "garrison", "marching", "positioned", "embarked", "repairing", "freighting", "fitting_out":
		default:
			continue
		}
		// This projection consumes silver only; crew does not affect ship pay.
		gross := combat.UnitUpkeep(u.typ, u.category, u.size, u.status, 0).Silver
		atPayer := u.payer != nil && u.location != nil && *u.payer == *u.location
		net := gross - combat.SoldCirculation(gross, atPayer, share)
		after := net
		if selected[id] {
			home += net
			field += gross
			after = gross
			if u.payer != nil {
				payers[*u.payer] = true
			} else if gross > 0 {
				unfunded = true
			}
		}
		if u.payer != nil {
			drain[*u.payer] += after
		}
	}
	var runway *float64
	if unfunded {
		zero := 0.0
		runway = &zero
	}
	for payer := range payers {
		if drain[payer] <= 0 {
			continue
		}
		var stock float64
		if err := h.pool.QueryRow(ctx, `SELECT COALESCE((SELECT GREATEST(0,LEAST(g.cap,g.amount+g.rate*GREATEST(0,w.current_tick-g.calc_tick))) FROM settlement_goods g WHERE g.settlement_id=$1 AND g.good_key='silver'),0) FROM worlds w WHERE w.id=$2`, payer, worldID).Scan(&stock); err != nil {
			return nil, err
		}
		value := stock / drain[payer]
		if runway == nil || value < *runway {
			runway = &value
		}
	}
	result["upkeep_silver_home"] = home
	result["upkeep_silver_field"] = field
	result["silver_payer_count"] = len(payers)
	if runway != nil {
		result["silver_runway_ticks"] = *runway
	}
	return result, nil
}
